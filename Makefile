build/Verifier.abi:
	solc --evm-version paris --abi RSA.G16.sol -o build

build/Verifier.bin:
	solc --evm-version paris --bin RSA.G16.sol -o build

verifier/verifier.go: build/Verifier.abi build/Verifier.bin
	abigen --abi build/Verifier.abi --pkg verifier --type Verifier --out verifier/verifier.go --bin build/Verifier.bin

.PHONY: clean
clean:
	rm -f build/Verifier.abi build/Pairing.abi build/Verifier.bin build/Pairing.bin verifier/verifier.go

.PHONY: all
all: verifier/verifier.go