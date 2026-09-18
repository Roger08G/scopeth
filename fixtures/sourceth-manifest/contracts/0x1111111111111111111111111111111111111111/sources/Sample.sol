// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract Sample {
    uint256 public value;

    function setValue(uint256 nextValue) external {
        value = nextValue;
    }
}
