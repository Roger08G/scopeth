package input

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxSourceFiles = 10_000
	MaxSourceBytes = 16 << 20
	MaxTotalBytes  = 256 << 20
	MaxJSONBytes   = 64 << 20
)

type Source struct {
	LogicalPath  string `json:"logical_path"`
	PhysicalPath string `json:"physical_path,omitempty"`
	Content      []byte `json:"-"`
	SHA256       string `json:"sha256"`
	Origin       string `json:"origin"`
	Address      string `json:"address,omitempty"`
}

type Snapshot struct {
	Kind            string            `json:"kind"`
	Root            string            `json:"root"`
	Sources         []Source          `json:"sources"`
	CompilerVersion string            `json:"compiler_version,omitempty"`
	Provenance      map[string]string `json:"provenance,omitempty"`
}

type standardJSON struct {
	Language string `json:"language"`
	Sources  map[string]struct {
		Content *string  `json:"content"`
		URLs    []string `json:"urls"`
	} `json:"sources"`
}

type buildInfo struct {
	SolcVersion string       `json:"solcVersion"`
	Input       standardJSON `json:"input"`
}

type sourcethManifest struct {
	SchemaVersion int `json:"schema_version"`
	Tool          struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"tool"`
	Run struct {
		ID string `json:"id"`
	} `json:"run"`
	Input struct {
		ChainID any `json:"chain_id_observed"`
	} `json:"input"`
	Contracts []struct {
		Address      string `json:"address"`
		Role         string `json:"role"`
		SourceStatus string `json:"source_status"`
		Files        []struct {
			RelativePath string `json:"relative_path"`
			SizeBytes    int64  `json:"size_bytes"`
			SHA256       string `json:"sha256"`
		} `json:"files"`
	} `json:"contracts"`
}

func Discover(path string) (Snapshot, error) {
	if strings.TrimSpace(path) == "" {
		return Snapshot{}, errors.New("la ruta de entrada está vacía")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolver entrada: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Snapshot{}, fmt.Errorf("abrir entrada: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Snapshot{}, errors.New("UNSAFE_PATH: la entrada no puede ser un enlace simbólico")
	}
	if info.IsDir() {
		return discoverDirectory(abs)
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, errors.New("UNSAFE_PATH: la entrada debe ser un archivo regular o directorio")
	}
	switch strings.ToLower(filepath.Ext(abs)) {
	case ".sol":
		source, err := readStableSource(abs, filepath.Base(abs), "file", "")
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Kind: "solidity_file", Root: filepath.Dir(abs), Sources: []Source{source}}, nil
	case ".json":
		return discoverJSON(abs)
	default:
		return Snapshot{}, errors.New("INPUT_INVALID: se admite un .sol, un directorio o un JSON compatible")
	}
}

func DiscoverSourcethManifest(path string) (Snapshot, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Snapshot{}, err
	}
	data, err := readLimited(abs, MaxJSONBytes)
	if err != nil {
		return Snapshot{}, err
	}
	var manifest sourcethManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Snapshot{}, fmt.Errorf("manifiesto Sourceth inválido: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Tool.Name != "sourceth" {
		return Snapshot{}, errors.New("INPUT_INVALID: el archivo no es un manifiesto Sourceth schema_version 1")
	}
	base := filepath.Dir(abs)
	var sources []Source
	seen := make(map[string]struct{})
	total := 0
	for _, contract := range manifest.Contracts {
		if contract.SourceStatus != "downloaded" && contract.SourceStatus != "reused" {
			continue
		}
		address := strings.ToLower(strings.TrimSpace(contract.Address))
		if !safeSegment(address) {
			return Snapshot{}, errors.New("UNSAFE_PATH: dirección no segura en el manifiesto Sourceth")
		}
		for _, entry := range contract.Files {
			rel, err := cleanLogical(entry.RelativePath)
			if err != nil {
				return Snapshot{}, fmt.Errorf("UNSAFE_PATH: archivo Sourceth %q: %w", entry.RelativePath, err)
			}
			physical, sourceRelative, err := sourcethSourcePath(base, address, rel)
			if err != nil {
				return Snapshot{}, err
			}
			logical := filepath.ToSlash(filepath.Join("sourceth", address, filepath.FromSlash(sourceRelative)))
			if _, exists := seen[logical]; exists {
				return Snapshot{}, fmt.Errorf("INPUT_INVALID: ruta lógica duplicada: %s", logical)
			}
			source, err := readStableSource(physical, logical, "sourceth_manifest", address)
			if err != nil {
				return Snapshot{}, err
			}
			if entry.SizeBytes != int64(len(source.Content)) || !strings.EqualFold(entry.SHA256, source.SHA256) {
				return Snapshot{}, fmt.Errorf("INPUT_INVALID: integridad Sourceth incorrecta para %s", rel)
			}
			total += len(source.Content)
			if total > MaxTotalBytes || len(sources) >= MaxSourceFiles {
				return Snapshot{}, errors.New("RESOURCE_LIMIT: el conjunto Sourceth supera los límites")
			}
			seen[logical] = struct{}{}
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return Snapshot{}, errors.New("INPUT_INVALID: el manifiesto Sourceth no contiene fuentes disponibles")
	}
	sortSources(sources)
	return Snapshot{
		Kind:    "sourceth_manifest",
		Root:    base,
		Sources: sources,
		Provenance: map[string]string{
			"sourceth_run_id":       manifest.Run.ID,
			"sourceth_tool_version": manifest.Tool.Version,
		},
	}, nil
}

func sourcethSourcePath(base, address, relative string) (string, string, error) {
	parts := strings.Split(relative, "/")
	var physical, sourceRelative, root string
	if len(parts) >= 4 && strings.EqualFold(parts[0], "contracts") && strings.EqualFold(parts[1], address) && parts[2] == "sources" {
		sourceRelative = strings.Join(parts[3:], "/")
		physical = filepath.Join(base, filepath.FromSlash(relative))
		root = filepath.Join(base, filepath.FromSlash(strings.Join(parts[:3], "/")))
	} else {
		// Compatibilidad con manifiestos preliminares que guardaban la ruta relativa a sources/.
		sourceRelative = relative
		root = filepath.Join(base, "contracts", address, "sources")
		physical = filepath.Join(root, filepath.FromSlash(relative))
	}
	if _, err := cleanLogical(sourceRelative); err != nil {
		return "", "", fmt.Errorf("UNSAFE_PATH: ruta Sourceth sin archivo fuente válido: %w", err)
	}
	if err := ensureWithin(root, physical); err != nil {
		return "", "", err
	}
	return physical, sourceRelative, nil
}

func discoverDirectory(root string) (Snapshot, error) {
	var sources []Source
	total := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != root && entry.IsDir() {
			name := strings.ToLower(entry.Name())
			if name == ".git" || name == ".scopeth" || name == "analysis" || name == "results" {
				return filepath.SkipDir
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("UNSAFE_PATH: enlace simbólico rechazado: %s", path)
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".sol" {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("UNSAFE_PATH: archivo especial rechazado: %s", path)
		}
		if len(sources) >= MaxSourceFiles {
			return errors.New("RESOURCE_LIMIT: demasiados archivos Solidity")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		logical, err := cleanLogical(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		source, err := readStableSource(path, logical, "directory", "")
		if err != nil {
			return err
		}
		total += len(source.Content)
		if total > MaxTotalBytes {
			return errors.New("RESOURCE_LIMIT: las fuentes superan 256 MiB")
		}
		sources = append(sources, source)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	if len(sources) == 0 {
		return Snapshot{}, errors.New("INPUT_INVALID: no se encontraron archivos .sol")
	}
	sortSources(sources)
	return Snapshot{Kind: "directory", Root: root, Sources: sources}, nil
}

func discoverJSON(path string) (Snapshot, error) {
	data, err := readLimited(path, MaxJSONBytes)
	if err != nil {
		return Snapshot{}, err
	}
	var probe struct {
		Tool struct {
			Name string `json:"name"`
		} `json:"tool"`
		Input json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal(data, &probe)
	if probe.Tool.Name == "sourceth" {
		return DiscoverSourcethManifest(path)
	}
	var standard standardJSON
	kind := "standard_json"
	compilerVersion := ""
	if len(probe.Input) > 0 && string(probe.Input) != "null" {
		var build buildInfo
		if err := json.Unmarshal(data, &build); err != nil {
			return Snapshot{}, fmt.Errorf("build-info inválido: %w", err)
		}
		standard = build.Input
		compilerVersion = build.SolcVersion
		kind = "build_info"
	} else if err := json.Unmarshal(data, &standard); err != nil {
		return Snapshot{}, fmt.Errorf("Standard JSON inválido: %w", err)
	}
	if standard.Language != "" && standard.Language != "Solidity" {
		return Snapshot{}, fmt.Errorf("INPUT_INVALID: lenguaje no soportado: %s", standard.Language)
	}
	var sources []Source
	total := 0
	for logicalRaw, item := range standard.Sources {
		logical, err := cleanLogical(logicalRaw)
		if err != nil {
			return Snapshot{}, fmt.Errorf("UNSAFE_PATH: source unit %q: %w", logicalRaw, err)
		}
		if item.Content == nil {
			if len(item.URLs) > 0 {
				return Snapshot{}, fmt.Errorf("INPUT_INVALID: URLs remotas no admitidas para %s", logical)
			}
			return Snapshot{}, fmt.Errorf("INPUT_INVALID: falta content para %s", logical)
		}
		content := []byte(*item.Content)
		if len(content) > MaxSourceBytes {
			return Snapshot{}, fmt.Errorf("RESOURCE_LIMIT: %s supera 16 MiB", logical)
		}
		total += len(content)
		if total > MaxTotalBytes || len(sources) >= MaxSourceFiles {
			return Snapshot{}, errors.New("RESOURCE_LIMIT: Standard JSON supera los límites")
		}
		digest := sha256.Sum256(content)
		sources = append(sources, Source{LogicalPath: logical, Content: content, SHA256: hex.EncodeToString(digest[:]), Origin: kind})
	}
	if len(sources) == 0 {
		return Snapshot{}, errors.New("INPUT_INVALID: Standard JSON sin fuentes")
	}
	sortSources(sources)
	return Snapshot{Kind: kind, Root: filepath.Dir(path), Sources: sources, CompilerVersion: compilerVersion}, nil
}

func readStableSource(path, logical, origin, address string) (Source, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return Source{}, fmt.Errorf("leer %s: %w", logical, err)
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return Source{}, fmt.Errorf("UNSAFE_PATH: fuente no regular: %s", logical)
	}
	if before.Size() > MaxSourceBytes {
		return Source{}, fmt.Errorf("RESOURCE_LIMIT: %s supera 16 MiB", logical)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("leer %s: %w", logical, err)
	}
	after, err := os.Lstat(path)
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return Source{}, fmt.Errorf("INPUT_INVALID: %s cambió durante el snapshot", logical)
	}
	digest := sha256.Sum256(data)
	return Source{LogicalPath: logical, PhysicalPath: path, Content: data, SHA256: hex.EncodeToString(digest[:]), Origin: origin, Address: address}, nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("UNSAFE_PATH: JSON no regular")
	}
	if info.Size() > limit {
		return nil, errors.New("RESOURCE_LIMIT: JSON demasiado grande")
	}
	return os.ReadFile(path)
}

func cleanLogical(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || filepath.IsAbs(value) || strings.Contains(value, "\x00") {
		return "", errors.New("ruta vacía, absoluta o inválida")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("ruta con segmento inseguro")
		}
	}
	return strings.Join(parts, "/"), nil
}

func safeSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	return !strings.ContainsAny(value, "/\\:\x00")
}

func ensureWithin(root, candidate string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("UNSAFE_PATH: traversal fuera de la raíz permitida")
	}
	return nil
}

func sortSources(sources []Source) {
	sort.Slice(sources, func(i, j int) bool { return sources[i].LogicalPath < sources[j].LogicalPath })
}
