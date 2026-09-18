// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IERC721Like {
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
    function setApprovalForAll(address operator, bool approved) external;
}

interface IERC1155Like {
    function safeBatchTransferFrom(
        address from,
        address to,
        uint256[] calldata ids,
        uint256[] calldata amounts,
        bytes calldata data
    ) external;
}

interface ICallback {
    function ping(uint256 value) external returns (uint256);
}

abstract contract Left {
    function hook(uint256 value) public virtual returns (uint256) {
        return value + 1;
    }
}

abstract contract Right {
    function hook(uint256 value) public virtual returns (uint256) {
        return value + 2;
    }
}

contract Advanced is Left, Right {
    struct Account {
        uint128 balance;
        uint128 nonce;
    }

    mapping(address account => Account data) internal accounts;
    IERC721Like public immutable nft;
    IERC1155Like public immutable multiToken;
    bool public paused;
    uint256[] internal queue;
    uint256 transient transientCounter;

    event Transfer(address indexed from, address indexed to, uint256 amount);
    event CallbackFailed(bytes reason);

    modifier whenActiveTwice() {
        require(!paused, "paused");
        _;
        _;
    }

    modifier bumpAccountNonce(Account storage account) {
        account.nonce++;
        _;
    }

    constructor(IERC721Like nft_, IERC1155Like multiToken_) {
        nft = nft_;
        multiToken = multiToken_;
    }

    function hook(uint256 value) public override(Left, Right) returns (uint256) {
        return super.hook(value);
    }

    function overloaded(uint256 value) external pure returns (uint256) {
        return value;
    }

    function overloaded(address value) external pure returns (address) {
        return value;
    }

    function updateThroughAlias(uint128 amount) external {
        Account storage account = accounts[msg.sender];
        account.balance += amount;
        account.nonce++;
    }

    function updateThroughInternalAlias(uint128 amount) external {
        Account storage account = accounts[msg.sender];
        _relayAccount(account, amount);
    }

    function _relayAccount(Account storage account, uint128 amount) internal {
        _applyAccount(account, amount);
    }

    function _applyAccount(Account storage account, uint128 amount) internal {
        account.balance += amount;
        account.nonce++;
    }

    function updateThroughRecursiveAlias(uint128 amount, uint256 depth) external {
        Account storage account = accounts[msg.sender];
        _recursiveApply(account, amount, depth);
    }

    function _recursiveApply(
        Account storage account,
        uint128 amount,
        uint256 depth
    ) internal {
        account.balance += amount;
        if (depth > 0) _recursiveApply(account, amount, depth - 1);
    }

    function bumpTransient() external returns (uint256) {
        transientCounter++;
        return transientCounter;
    }

    function mutateArray(uint256 value) external {
        queue.push(value);
        queue[0] = value;
        queue.pop();
    }

    function repeatedEffect(uint128 amount) external whenActiveTwice {
        accounts[msg.sender].balance += amount;
    }

    function modifierThroughStorageBinding()
        external
        bumpAccountNonce(accounts[msg.sender])
    {}

    function loopAndShortCircuit(uint256[] calldata values) external returns (uint256 total) {
        for (uint256 i; i < values.length; ++i) {
            if (values[i] == 0 || paused) continue;
            total += values[i];
            if (total > type(uint128).max) break;
        }
        accounts[msg.sender].balance = uint128(total);
    }

    function callback(ICallback target, uint256 value) external returns (uint256) {
        try target.ping(value) returns (uint256 answer) {
            return answer;
        } catch (bytes memory reason) {
            emit CallbackFailed(reason);
            return 0;
        }
    }

    function callThis(uint256 value) external returns (uint256) {
        return this.overloaded(value);
    }

    function recurse(uint256 value) external pure returns (uint256) {
        return _recurse(value);
    }

    function _recurse(uint256 value) internal pure returns (uint256) {
        if (value == 0) return 0;
        return 1 + _recurse(value - 1);
    }

    function moveNFT(address from, address to, uint256 tokenId) external {
        nft.safeTransferFrom(from, to, tokenId);
    }

    function authorizeNFT(address operator, bool approved) external {
        nft.setApprovalForAll(operator, approved);
    }

    function moveBatch(
        address from,
        address to,
        uint256[] calldata ids,
        uint256[] calldata amounts
    ) external {
        multiToken.safeBatchTransferFrom(from, to, ids, amounts, "");
    }

    function spendExistingNative(address payable recipient, uint256 amount) external {
        (bool ok,) = recipient.call{value: amount}("");
        require(ok, "native call failed");
    }

    function staticProbe(address target, bytes calldata data) external view returns (bytes memory) {
        (bool ok, bytes memory result) = target.staticcall(data);
        require(ok, "staticcall failed");
        return result;
    }

    function transfer(address to, uint256 amount) external {
        emit Transfer(msg.sender, to, amount);
    }
}
