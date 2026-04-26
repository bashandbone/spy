// Solidity sample
// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface ICounter {
    function increment(uint256 n) external returns (uint256);
    function value() external view returns (uint256);
}

contract Counter is ICounter {
    uint256 private _value;
    address public immutable owner;

    event Incremented(address indexed by, uint256 amount, uint256 newValue);

    error NotOwner(address caller);

    modifier onlyOwner() {
        if (msg.sender != owner) {
            revert NotOwner(msg.sender);
        }
        _;
    }

    constructor() {
        owner = msg.sender;
        _value = 0;
    }

    function increment(uint256 n) external onlyOwner returns (uint256) {
        _value += n;
        emit Incremented(msg.sender, n, _value);
        return _value;
    }

    function value() external view returns (uint256) {
        return _value;
    }
}
