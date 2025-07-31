build-groth16/Verifier.abi:
	solc --overwrite --evm-version paris --abi RSA.G16.sol -o build-groth16

build-groth16/Verifier.bin:
	solc --overwrite --evm-version paris --bin RSA.G16.sol -o build-groth16

verifier_groth16/verifier.go: build-groth16/Verifier.abi build-groth16/Verifier.bin
	abigen --abi build-groth16/Verifier.abi --pkg verifier_groth16 --type VerifierGroth16 --out verifier_groth16/verifier.go --bin build-groth16/Verifier.bin

build-plonk/Verifier.abi:
	solc --overwrite --evm-version paris --abi RSA.PLONK.sol -o build-plonk

build-plonk/Verifier.bin:
	solc --overwrite --evm-version paris --bin RSA.PLONK.sol -o build-plonk

verifier_plonk/verifier.go: build-plonk/Verifier.abi build-plonk/Verifier.bin
	abigen --abi build-plonk/PlonkVerifier.abi --pkg verifier_plonk --type VerifierPlonk --out verifier_plonk/verifier.go --bin build-plonk/PlonkVerifier.bin

.PHONY: clean
clean:
	rm -f build-groth16/Verifier.abi build-groth16/Verifier.bin verifier_groth16/verifier.go
	rm -f build-plonk/PlonkVerifier.abi build-plonk/PlonkVerifier.bin verifier_plonk/verifier.go
	git restore verifier_groth16/verifier.go verifier_plonk/verifier.go

.PHONY: clean-setup
clean-setup:
	rm -f RSA.G16.ccs RSA.G16.vk RSA.G16.pk RSA.G16.sol
	rm -f RSA.PLONK.ccs RSA.PLONK.vk RSA.PLONK.pk RSA.PLONK.sol
	rm -rf build-groth16 build-plonk

.PHONY: all
all: verifier_groth16/verifier.go verifier_plonk/verifier.go