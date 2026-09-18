// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IERC20Surface {
    function transfer(address to, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function approve(address spender, uint256 amount) external returns (bool);
    function permit(
        address owner,
        address spender,
        uint256 value,
        uint256 deadline,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external;
}

interface IERC721Surface {
    function transferFrom(address from, address to, uint256 tokenId) external;
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
    function setApprovalForAll(address operator, bool approved) external;
}

interface IERC1155Surface {
    function safeBatchTransferFrom(
        address from,
        address to,
        uint256[] calldata ids,
        uint256[] calldata amounts,
        bytes calldata data
    ) external;
}

interface IERC4626Surface {
    function deposit(uint256 assets, address receiver) external returns (uint256 shares);
    function redeem(uint256 shares, address receiver, address owner) external returns (uint256 assets);
}

interface IWETHSurface {
    function deposit() external payable;
    function withdraw(uint256 amount) external;
}

interface IOracleSurface {
    function latestRoundData()
        external
        view
        returns (uint80, int256, uint256, uint256, uint80);
}

interface ICustomSurface {
    function transfer(address to, uint256 amount) external;
}

library SafeERC20Surface {
    function safeTransfer(IERC20Surface token, address to, uint256 amount) internal {
        require(token.transfer(to, amount), "safe transfer");
    }
}

contract ReceiverSurface {
    constructor() payable {}
}

contract AssetsSurface {
    using SafeERC20Surface for IERC20Surface;

    address public owner;
    address public treasury;
    address public implementation;
    IOracleSurface public oracle;
    uint256 public feeBps;
    bool public paused;
    mapping(address account => uint256 amount) public balances;

    IERC20Surface public immutable erc20;
    IERC721Surface public immutable erc721;
    IERC1155Surface public immutable erc1155;
    IERC4626Surface public immutable vault;
    IWETHSurface public immutable weth;
    ICustomSurface public immutable custom;

    modifier onlyOwner() {
        require(msg.sender == owner, "owner");
        _;
    }

    constructor(
        IERC20Surface erc20_,
        IERC721Surface erc721_,
        IERC1155Surface erc1155_,
        IERC4626Surface vault_,
        IWETHSurface weth_,
        ICustomSurface custom_,
        IOracleSurface oracle_
    ) {
        owner = msg.sender;
        erc20 = erc20_;
        erc721 = erc721_;
        erc1155 = erc1155_;
        vault = vault_;
        weth = weth_;
        custom = custom_;
        oracle = oracle_;
    }

    receive() external payable {}

    function checkedTransfer(address to, uint256 amount) external onlyOwner returns (bool) {
        return erc20.transfer(to, amount);
    }

    function requiredTransfer(address to, uint256 amount) external onlyOwner {
        require(erc20.transfer(to, amount), "transfer");
    }

    function uncheckedTransfer(address to, uint256 amount) external {
        erc20.transfer(to, amount);
    }

    function wrappedTransfer(address to, uint256 amount) external onlyOwner {
        erc20.safeTransfer(to, amount);
    }

    function pull(address from, uint256 amount) external {
        require(erc20.transferFrom(from, address(this), amount), "pull");
        balances[from] += amount;
    }

    function authorize(address spender, uint256 amount) external onlyOwner {
        require(erc20.approve(spender, amount), "approve");
    }

    function permitToken(
        address tokenOwner,
        address spender,
        uint256 amount,
        uint256 deadline,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external {
        erc20.permit(tokenOwner, spender, amount, deadline, v, r, s);
    }

    function moveNFT(address from, address to, uint256 tokenId) external onlyOwner {
        erc721.safeTransferFrom(from, to, tokenId);
    }

    function approveNFT(address operator, bool approved) external onlyOwner {
        erc721.setApprovalForAll(operator, approved);
    }

    function moveBatch(
        address from,
        address to,
        uint256[] calldata ids,
        uint256[] calldata amounts,
        bytes calldata data
    ) external onlyOwner {
        erc1155.safeBatchTransferFrom(from, to, ids, amounts, data);
    }

    function vaultRoundTrip(uint256 assets, uint256 shares, address receiver) external {
        vault.deposit(assets, receiver);
        vault.redeem(shares, receiver, msg.sender);
    }

    function wrapAndUnwrap(uint256 amount) external payable {
        weth.deposit{value: msg.value}();
        weth.withdraw(amount);
    }

    function sendNative(address payable to, uint256 amount) external onlyOwner {
        bool sent = to.send(amount);
        require(sent, "send");
        to.transfer(amount);
        (bool called,) = to.call{value: amount}("");
        require(called, "call");
    }

    function deployWithValue(uint256 amount) external onlyOwner returns (address) {
        return address(new ReceiverSurface{value: amount}());
    }

    function setTreasury(address nextTreasury) external onlyOwner {
        treasury = nextTreasury;
    }

    function setOracle(IOracleSurface nextOracle) external onlyOwner {
        oracle = nextOracle;
    }

    function setFee(uint256 nextFeeBps) external onlyOwner {
        require(nextFeeBps <= 1_000, "fee");
        feeBps = nextFeeBps;
    }

    function pause(bool value) external onlyOwner {
        paused = value;
    }

    function upgradeTo(address nextImplementation) external onlyOwner {
        implementation = nextImplementation;
    }

    function quote(uint256 amount) external view returns (uint64) {
        (, int256 answer,, uint256 updatedAt,) = oracle.latestRoundData();
        require(answer > 0 && updatedAt <= block.timestamp, "oracle");
        return uint64(amount / uint256(answer));
    }

    function batchAccount(address[] calldata accounts, uint256 amount) external {
        for (uint256 index; index < accounts.length; ++index) {
            unchecked {
                balances[accounts[index]] += amount;
            }
        }
    }

    function batchTokens(address[] calldata recipients, uint256 amount) external onlyOwner {
        for (uint256 index; index < recipients.length; ++index) {
            require(erc20.transfer(recipients[index], amount), "transfer");
        }
    }

    function delegate(address target, bytes calldata data) external onlyOwner returns (bytes memory) {
        (bool ok, bytes memory result) = target.delegatecall(data);
        require(ok, "delegate");
        return result;
    }

    function customNamedTransfer(address to, uint256 amount) external {
        custom.transfer(to, amount);
    }

    function assemblySurface(bytes32 slot, address target, uint256 value) external onlyOwner {
        assembly {
            let current := sload(slot)
            sstore(slot, add(current, value))
            tstore(slot, value)
            let transientValue := tload(slot)
            pop(staticcall(gas(), target, 0, 0, 0, 0))
            pop(call(gas(), target, transientValue, 0, 0, 0, 0))
            pop(create2(0, 0, 0, slot))
            log0(0, 0)
            switch value
            case 0 { pop(0) }
            default { pop(1) }
        }
    }

    function destroy(address payable recipient) external onlyOwner {
        selfdestruct(recipient);
    }
}
