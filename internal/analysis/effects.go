package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/Roger08G/scopeth/internal/model"
)

type effectSet map[string]model.StorageEffect

func newEffectSet() effectSet { return make(effectSet) }

func effectSetFrom(values []model.StorageEffect) effectSet {
	result := newEffectSet()
	for _, value := range values {
		result.add(value)
	}
	return result
}

func (s effectSet) add(effect model.StorageEffect) bool {
	if effect.RootKind == "" {
		effect.RootKind = "state"
	}
	key := effectKey(effect)
	current, exists := s[key]
	if !exists {
		s[key] = effect
		return true
	}
	merged := current
	merged.Access = mergeAccess(current.Access, effect.Access)
	merged.Resolution = worseResolution(current.Resolution, effect.Resolution)
	merged.ViaAlias = current.ViaAlias || effect.ViaAlias
	if merged != current {
		s[key] = merged
		return true
	}
	return false
}

func (s effectSet) get(effect model.StorageEffect) (model.StorageEffect, bool) {
	value, ok := s[effectKey(effect)]
	return value, ok
}

func (s effectSet) values() []model.StorageEffect {
	result := make([]model.StorageEffect, 0, len(s))
	for _, effect := range s {
		result = append(result, effect)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := effectKey(result[i]), effectKey(result[j])
		if left != right {
			return left < right
		}
		return result[i].Access < result[j].Access
	})
	return result
}

func effectKey(effect model.StorageEffect) string {
	return strings.Join([]string{effect.RootKind, effect.VariableID, effect.Path}, "\x00")
}

func storageEffect(access model.StateAccess) model.StorageEffect {
	rootKind := access.RootKind
	if rootKind == "" {
		rootKind = "state"
	}
	path := access.Path
	if path == "" {
		path = access.Variable
	}
	return model.StorageEffect{
		RootKind: rootKind, VariableID: access.VariableID, Variable: access.Variable,
		Path: path, Access: access.Access, Resolution: normalizedResolution(access.Resolution),
		ViaAlias: access.ViaAlias,
	}
}

func instantiateEffect(effect model.StorageEffect, bindings []model.StorageBinding) (model.StorageEffect, bool) {
	if effect.RootKind == "state" {
		return effect, true
	}
	if effect.RootKind != "storage_parameter" {
		return model.StorageEffect{}, false
	}
	for _, binding := range bindings {
		if binding.ParameterID != effect.VariableID {
			continue
		}
		if binding.ArgumentRootKind != "state" && binding.ArgumentRootKind != "storage_parameter" {
			return model.StorageEffect{}, false
		}
		suffix, ok := storageSuffix(effect.Path, effect.Variable)
		if !ok {
			return model.StorageEffect{}, false
		}
		path := binding.ArgumentPath
		if path == "" {
			path = binding.ArgumentVariable
		}
		return model.StorageEffect{
			RootKind: binding.ArgumentRootKind, VariableID: binding.ArgumentVariableID,
			Variable: binding.ArgumentVariable, Path: path + suffix, Access: effect.Access,
			Resolution: worseResolution(effect.Resolution, binding.Resolution),
			ViaAlias:   effect.ViaAlias || binding.ViaAlias,
		}, binding.ArgumentVariableID != ""
	}
	return model.StorageEffect{}, false
}

func storageSuffix(path, root string) (string, bool) {
	if path == "" || path == root {
		return "", true
	}
	if strings.HasPrefix(path, root+".") || strings.HasPrefix(path, root+"[") {
		return strings.TrimPrefix(path, root), true
	}
	return "", false
}

func mergeAccess(left, right string) string {
	leftMask, rightMask := accessMask(left), accessMask(right)
	return accessFromMask(leftMask | rightMask)
}

func missingAccess(current model.StorageEffect, exists bool, wanted string) string {
	if !exists {
		return wanted
	}
	missing := accessMask(wanted) &^ accessMask(current.Access)
	return accessFromMask(missing)
}

func accessMask(value string) int {
	switch value {
	case "read":
		return 1
	case "write":
		return 2
	case "read_write":
		return 3
	default:
		return 0
	}
}

func accessFromMask(mask int) string {
	switch mask {
	case 1:
		return "read"
	case 2:
		return "write"
	case 3:
		return "read_write"
	default:
		return ""
	}
}

func worseResolution(left, right string) string {
	left, right = normalizedResolution(left), normalizedResolution(right)
	if resolutionRank(right) > resolutionRank(left) {
		return right
	}
	return left
}

func normalizedResolution(value string) string {
	switch value {
	case "resolved", "partial", "unknown":
		return value
	default:
		return "unknown"
	}
}

func resolutionRank(value string) int {
	switch value {
	case "resolved":
		return 0
	case "partial":
		return 1
	default:
		return 2
	}
}

func set() map[string]struct{} { return make(map[string]struct{}) }

func merge(destination, source map[string]struct{}) bool {
	changed := false
	for value := range source {
		if _, ok := destination[value]; !ok {
			destination[value], changed = struct{}{}, true
		}
	}
	return changed
}

func sorted(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func uniqueSorted(values []string) []string {
	result := set()
	for _, value := range values {
		result[value] = struct{}{}
	}
	return sorted(result)
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func hashID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}
