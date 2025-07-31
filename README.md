# Demo for deploying gnark on Ethereum

## Requirements

Install Solidity

    brew install solidity

Install Go:

    brew install golang

Install abigen:

    go install github.com/ethereum/go-ethereum/cmd/abigen@v1.16.1

## Running

1. Look at `circuit/circuit.go`, this is the circuit definition
2. Generate gnark internal representation of the circuit:

        go run main.go generate

3. View the solidity smart contract `RSA.G16.sol` and `RSA.PLONK.sol` (other files with same prefix are proving key, veriyfing key and compiled arithmetisation)
4. Generate ABI and bytecode:

        make all

5. Run the tests:

        go run main.go testGroth16
        go run main.go testPlonk

You can have a look at `main.go` on how to compile, deploy and run.
