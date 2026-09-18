// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IERC20 {
    function transfer(address to, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function approve(address spender, uint256 amount) external returns (bool);
}

abstract contract Owned {
    address public owner;

    error Unauthorized(address caller);

    constructor() {
        owner = msg.sender;
    }

    modifier onlyOwner() {
        if (msg.sender != owner) revert Unauthorized(msg.sender);
        _;
    }
}

contract Vault is Owned {
    IERC20 public immutable token;
    mapping(address account => uint256 balance) public balances;

    event Deposited(address indexed account, uint256 amount);
    event Withdrawn(address indexed account, uint256 amount);

    error InsufficientBalance(uint256 available, uint256 requested);

    constructor(IERC20 token_) {
        token = token_;
    }

    receive() external payable {}

    fallback() external payable {}

    function deposit(uint256 amount) external {
        require(amount > 0, "amount=0");
        require(token.transferFrom(msg.sender, address(this), amount), "transferFrom failed");
        _credit(msg.sender, amount);
        emit Deposited(msg.sender, amount);
    }

    function withdraw(uint256 amount) external {
        _debit(msg.sender, amount);
        require(token.transfer(msg.sender, amount), "transfer failed");
        emit Withdrawn(msg.sender, amount);
    }

    function _credit(address account, uint256 amount) internal {
        balances[account] += amount;
    }

    function _debit(address account, uint256 amount) internal {
        uint256 available = balances[account];
        if (available < amount) revert InsufficientBalance(available, amount);
        balances[account] = available - amount;
    }

    function approveSpender(address spender, uint256 amount) external onlyOwner {
        require(token.approve(spender, amount), "approve failed");
    }

    function sweepNative(address payable recipient, uint256 amount) external onlyOwner {
        recipient.transfer(amount);
    }

    function forwardNative(address payable recipient, uint256 amount) external onlyOwner {
        (bool ok,) = recipient.call{value: amount}("");
        require(ok, "native call failed");
    }

    function reviewDelegatecall(address target, bytes calldata data) external onlyOwner returns (bytes memory) {
        (bool ok, bytes memory result) = target.delegatecall(data);
        require(ok, "delegatecall failed");
        return result;
    }

    function assemblyRead(bytes32 slot) external view returns (bytes32 result) {
        assembly {
            result := sload(slot)
        }
    }
}

function double(uint256 value) pure returns (uint256) {
    return value * 2;
}
