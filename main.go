package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	fp_bn254 "github.com/consensys/gnark-crypto/ecc/bn254/fp"
	fr_bn254 "github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/backend/groth16"
	plonk "github.com/consensys/gnark/backend/plonk"
	"github.com/consensys/gnark/backend/solidity"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/frontend/cs/scs"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/emulated/emparams"
	"github.com/consensys/gnark/test/unsafekzg"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ivokub/smartcontract/circuit"
	"github.com/ivokub/smartcontract/verifier_groth16"
	"github.com/ivokub/smartcontract/verifier_plonk"
)

const (
	NAME_G16   = "RSA.G16"
	NAME_PLONK = "RSA.PLONK"
)

var (
	VKNAME_G16  = NAME_G16 + ".vk"
	PKNAME_G16  = NAME_G16 + ".pk"
	SOLNAME_G16 = NAME_G16 + ".sol"
	CCSNAME_G16 = NAME_G16 + ".ccs"

	VKNAME_PLONK  = NAME_PLONK + ".vk"
	PKNAME_PLONK  = NAME_PLONK + ".pk"
	SOLNAME_PLONK = NAME_PLONK + ".sol"
	CCSNAME_PLONK = NAME_PLONK + ".ccs"
)

const (
	NbPublicInputs = 5 // 1 native + 4 for a non-native element
	NbCommitments  = 1 // 1 commitment
)

var curve = ecc.BN254

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("subcommand 'generate', 'testGroth16' or 'testPlonk'")
		os.Exit(0)
	}
	switch args[0] {
	case "generate":
		if err := generateGroth16(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := generatePlonk(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "testGroth16":
		ev, err := setupGroth16()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := runGroth16(ev); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "testPlonk":
		ev, err := setupPlonk()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := runPlonk(ev); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	default:
		fmt.Println("unknown subcommand. valid commands 'generate', 'testGroth16', 'testPlonk'")
		os.Exit(0)
	}
	fmt.Println("OK!")
}

func generateGroth16() error {
	var circuit circuit.Circuit

	ccs, err := frontend.Compile(curve.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return err
	}
	pk, vk, err := groth16.Setup(ccs) // NB Unsafe, use MPC
	if err != nil {
		return err
	}

	fccs, err := os.Create(CCSNAME_G16)
	if err != nil {
		return err
	}
	defer fccs.Close()
	_, err = ccs.WriteTo(fccs)
	if err != nil {
		return err
	}

	fvk, err := os.Create(VKNAME_G16)
	if err != nil {
		return err
	}
	defer fvk.Close()
	_, err = vk.WriteRawTo(fvk)
	if err != nil {
		return err
	}

	fpk, err := os.Create(PKNAME_G16)
	if err != nil {
		return err
	}
	defer fpk.Close()
	_, err = pk.WriteRawTo(fpk)
	if err != nil {
		return err
	}

	fsol, err := os.Create(SOLNAME_G16)
	if err != nil {
		return err
	}
	defer fsol.Close()
	err = vk.ExportSolidity(fsol)
	if err != nil {
		return err
	}

	return nil
}

type ethVerifierGroth16 struct {
	// backend
	backend *backends.SimulatedBackend

	// verifier contract
	verifierContract *verifier_groth16.VerifierGroth16

	// groth16 gnark objects
	vk      groth16.VerifyingKey
	pk      groth16.ProvingKey
	circuit circuit.Circuit
	r1cs    constraint.ConstraintSystem
}

type ethVerifierPlonk struct {
	// backend
	backend *backends.SimulatedBackend

	// verifier contract
	verifierContract *verifier_plonk.VerifierPlonk

	// plonk gnark objects
	vk      plonk.VerifyingKey
	pk      plonk.ProvingKey
	circuit circuit.Circuit
	scs     constraint.ConstraintSystem
}

func setupGroth16() (*ethVerifierGroth16, error) {
	const gasLimit uint64 = 4712388

	// setup simulated backend
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("new key: %w", err)
	}
	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337))
	if err != nil {
		return nil, fmt.Errorf("new transactor: %w", err)
	}

	genesis := map[common.Address]core.GenesisAccount{
		auth.From: {Balance: big.NewInt(1000000000000000000)}, // 1 Eth
	}

	newbackend := backends.NewSimulatedBackend(genesis, gasLimit)

	// deploy verifier contract
	caddr, _, v, err := verifier_groth16.DeployVerifierGroth16(auth, newbackend)
	if err != nil {
		return nil, fmt.Errorf("new verifier: %w", err)
	}
	newbackend.Commit()
	fmt.Printf("deployed contract at %s\n", caddr)

	fccs, err := os.Open(CCSNAME_G16)
	if err != nil {
		return nil, fmt.Errorf("open ccs: %w", err)
	}
	defer fccs.Close()
	ccs := groth16.NewCS(curve)
	_, err = ccs.ReadFrom(fccs)
	if err != nil {
		return nil, fmt.Errorf("read ccs: %w", err)
	}
	fpk, err := os.Open(PKNAME_G16)
	if err != nil {
		return nil, fmt.Errorf("open pk: %w", err)
	}
	defer fpk.Close()
	pk := groth16.NewProvingKey(curve)
	_, err = pk.ReadFrom(fpk)
	if err != nil {
		return nil, fmt.Errorf("read pk: %w", err)
	}
	fvk, err := os.Open(VKNAME_G16)
	if err != nil {
		return nil, fmt.Errorf("open vk: %w", err)
	}
	defer fvk.Close()
	vk := groth16.NewVerifyingKey(curve)
	_, err = vk.ReadFrom(fvk)
	if err != nil {
		return nil, fmt.Errorf("read vk: %w", err)
	}
	return &ethVerifierGroth16{
		backend:          newbackend,
		verifierContract: v,
		vk:               vk,
		pk:               pk,
		circuit:          circuit.Circuit{},
		r1cs:             ccs,
	}, nil
}

func runGroth16(ev *ethVerifierGroth16) error {
	n := 15
	p, q := 3, 5
	n2 := 63
	p2, q2 := 7, 9
	assignment := circuit.Circuit{
		X:    p,
		Y:    q,
		Z:    n,
		EmuX: emulated.ValueOf[emparams.BN254Fr](p2),
		EmuY: emulated.ValueOf[emparams.BN254Fr](q2),
		EmuZ: emulated.ValueOf[emparams.BN254Fr](n2),
	}

	// witness creation
	witness, err := frontend.NewWitness(&assignment, curve.ScalarField())
	if err != nil {
		return fmt.Errorf("new witness: %w", err)
	}

	// prove
	proof, err := groth16.Prove(ev.r1cs, ev.pk, witness, solidity.WithProverTargetSolidityVerifier(backend.GROTH16))
	if err != nil {
		return fmt.Errorf("prove: %w", err)
	}

	// ensure gnark (Go) code verifies it
	publicWitness, err := witness.Public()
	if err != nil {
		return fmt.Errorf("new public witness: %w", err)
	}
	if err = groth16.Verify(proof, ev.vk, publicWitness, solidity.WithVerifierTargetSolidityVerifier(backend.GROTH16)); err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	// Set the proof as big inputs what the contract expects
	proofBytes := proof.(interface{ MarshalSolidity() []byte }).MarshalSolidity()
	var proofInts [8]*big.Int
	for i := range proofInts {
		proofInts[i] = new(big.Int).SetBytes(proofBytes[fp_bn254.Bytes*i : fp_bn254.Bytes*(i+1)])
	}

	// Set the commitments. This part is not necessary when the circuit does not use commitments
	var commitments [2 * NbCommitments]*big.Int
	var commitmentsPok [2]*big.Int
	for i := range commitments {
		commitments[i] = new(big.Int).SetBytes(proofBytes[fp_bn254.Bytes*len(proofInts)+4+i*fp_bn254.Bytes : fp_bn254.Bytes*len(proofInts)+4+(i+1)*fp_bn254.Bytes])
	}
	// Set the commitment pok
	for i := range commitmentsPok {
		commitmentsPok[i] = new(big.Int).SetBytes(proofBytes[fp_bn254.Bytes*(len(proofInts)+len(commitments))+4+i*fp_bn254.Bytes : fp_bn254.Bytes*(len(proofInts)+len(commitments))+4+(i+1)*fp_bn254.Bytes])
	}

	// now convert the public inputs.
	publicWitnessVector := publicWitness.Vector()
	tPublicWitnessVector, ok := publicWitnessVector.(fr_bn254.Vector)
	if !ok {
		return fmt.Errorf("expected fr_bn254.Vector, got %T", publicWitnessVector)
	}

	var publicInputs [NbPublicInputs]*big.Int
	if len(tPublicWitnessVector) != NbPublicInputs {
		return fmt.Errorf("expected %d public inputs, got %d", NbPublicInputs, len(tPublicWitnessVector))
	}
	for i, e := range tPublicWitnessVector {
		publicInputs[i] = e.BigInt(new(big.Int))
	}

	for i := range proofInts {
		fmt.Printf("proof[%d] = %s\n", i, proofInts[i].Text(16))
	}
	for i := range commitments {
		fmt.Printf("commitments[%d] = %s\n", i, commitments[i].Text(16))
	}
	for i := range commitmentsPok {
		fmt.Printf("commitmentsPok[%d] = %s\n", i, commitmentsPok[i].Text(16))
	}
	for i := range publicInputs {
		fmt.Printf("publicInputs[%d] = %s\n", i, publicInputs[i].Text(10))
	}

	// call the contract
	err = ev.verifierContract.VerifyProof(nil, proofInts, commitments, commitmentsPok, publicInputs)
	if err != nil {
		return fmt.Errorf("calling verifier: %w", err)
	}

	// (wrong) public witness
	var wrongPublicInput [NbPublicInputs]*big.Int
	for i := 0; i < NbPublicInputs; i++ {
		wrongPublicInput[i] = new(big.Int).SetUint64(999)
	}

	// call the contract should fail
	err = ev.verifierContract.VerifyProof(nil, proofInts, commitments, commitmentsPok, wrongPublicInput)
	if err == nil {
		return fmt.Errorf("call verifier with wrong input should have failed")
	}
	return nil
}

func generatePlonk() error {
	var circuit circuit.Circuit

	ccs, err := frontend.Compile(curve.ScalarField(), scs.NewBuilder, &circuit)
	if err != nil {
		return err
	}
	srsCan, srsLag, err := unsafekzg.NewSRS(ccs, unsafekzg.WithToxicSeed([]byte("toxic_seed"))) // NB unsafe, use MPC or reuse existing KZG SRS
	if err != nil {
		return err
	}
	pk, vk, err := plonk.Setup(ccs, srsCan, srsLag)
	if err != nil {
		return err
	}

	fccs, err := os.Create(CCSNAME_PLONK)
	if err != nil {
		return err
	}
	defer fccs.Close()
	_, err = ccs.WriteTo(fccs)
	if err != nil {
		return err
	}

	fvk, err := os.Create(VKNAME_PLONK)
	if err != nil {
		return err
	}
	defer fvk.Close()
	_, err = vk.WriteRawTo(fvk)
	if err != nil {
		return err
	}

	fpk, err := os.Create(PKNAME_PLONK)
	if err != nil {
		return err
	}
	defer fpk.Close()
	_, err = pk.WriteRawTo(fpk)
	if err != nil {
		return err
	}

	fsol, err := os.Create(SOLNAME_PLONK)
	if err != nil {
		return err
	}
	defer fsol.Close()
	err = vk.ExportSolidity(fsol)
	if err != nil {
		return err
	}

	return nil
}

func setupPlonk() (*ethVerifierPlonk, error) {
	const gasLimit uint64 = 4712388

	// setup simulated backend
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("new key: %w", err)
	}
	auth, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(1337))
	if err != nil {
		return nil, fmt.Errorf("new transactor: %w", err)
	}

	genesis := map[common.Address]core.GenesisAccount{
		auth.From: {Balance: big.NewInt(1000000000000000000)}, // 1 Eth
	}

	newbackend := backends.NewSimulatedBackend(genesis, gasLimit)

	// deploy verifier contract
	caddr, _, v, err := verifier_plonk.DeployVerifierPlonk(auth, newbackend)
	if err != nil {
		return nil, fmt.Errorf("new verifier: %w", err)
	}
	newbackend.Commit()
	fmt.Printf("deployed contract at %s\n", caddr)

	fccs, err := os.Open(CCSNAME_PLONK)
	if err != nil {
		return nil, fmt.Errorf("open ccs: %w", err)
	}
	defer fccs.Close()
	ccs := plonk.NewCS(curve)
	_, err = ccs.ReadFrom(fccs)
	if err != nil {
		return nil, fmt.Errorf("read ccs: %w", err)
	}
	fpk, err := os.Open(PKNAME_PLONK)
	if err != nil {
		return nil, fmt.Errorf("open pk: %w", err)
	}
	defer fpk.Close()
	pk := plonk.NewProvingKey(curve)
	_, err = pk.ReadFrom(fpk)
	if err != nil {
		return nil, fmt.Errorf("read pk: %w", err)
	}
	fvk, err := os.Open(VKNAME_PLONK)
	if err != nil {
		return nil, fmt.Errorf("open vk: %w", err)
	}
	defer fvk.Close()
	vk := plonk.NewVerifyingKey(curve)
	_, err = vk.ReadFrom(fvk)
	if err != nil {
		return nil, fmt.Errorf("read vk: %w", err)
	}
	return &ethVerifierPlonk{
		backend:          newbackend,
		verifierContract: v,
		vk:               vk,
		pk:               pk,
		circuit:          circuit.Circuit{},
		scs:              ccs,
	}, nil
}

func runPlonk(ev *ethVerifierPlonk) error {
	n := 15
	p, q := 3, 5
	n2 := 63
	p2, q2 := 7, 9
	assignment := circuit.Circuit{
		X:    p,
		Y:    q,
		Z:    n,
		EmuX: emulated.ValueOf[emparams.BN254Fr](p2),
		EmuY: emulated.ValueOf[emparams.BN254Fr](q2),
		EmuZ: emulated.ValueOf[emparams.BN254Fr](n2),
	}

	// witness creation
	witness, err := frontend.NewWitness(&assignment, curve.ScalarField())
	if err != nil {
		return fmt.Errorf("new witness: %w", err)
	}

	// prove
	proof, err := plonk.Prove(ev.scs, ev.pk, witness, solidity.WithProverTargetSolidityVerifier(backend.PLONK))
	if err != nil {
		return fmt.Errorf("prove: %w", err)
	}

	// ensure gnark (Go) code verifies it
	publicWitness, err := witness.Public()
	if err != nil {
		return fmt.Errorf("new public witness: %w", err)
	}
	if err = plonk.Verify(proof, ev.vk, publicWitness, solidity.WithVerifierTargetSolidityVerifier(backend.PLONK)); err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	// prepare the proof for the contract
	proofBytes := proof.(interface{ MarshalSolidity() []byte }).MarshalSolidity()

	// now convert the public inputs.
	publicWitnessVector := publicWitness.Vector()
	tPublicWitnessVector, ok := publicWitnessVector.(fr_bn254.Vector)
	if !ok {
		return fmt.Errorf("expected fr_bn254.Vector, got %T", publicWitnessVector)
	}

	publicInputs := make([]*big.Int, ev.scs.GetNbPublicVariables())
	if len(tPublicWitnessVector) != len(publicInputs) {
		return fmt.Errorf("expected %d public inputs, got %d", NbPublicInputs, len(tPublicWitnessVector))
	}
	for i, e := range tPublicWitnessVector {
		publicInputs[i] = e.BigInt(new(big.Int))
	}

	fmt.Printf("proof %d: %x\n", len(proofBytes), proofBytes)
	for i := range publicInputs {
		fmt.Printf("publicInputs[%d] = %s\n", i, publicInputs[i].Text(10))
	}

	// call the contract
	ok, err = ev.verifierContract.Verify(nil, proofBytes, publicInputs)
	if err != nil {
		return fmt.Errorf("calling verifier: %w", err)
	}
	if !ok {
		return fmt.Errorf("verifier returned false")
	}

	// (wrong) public witness
	wrongPublicInput := make([]*big.Int, ev.scs.GetNbPublicVariables())
	for i := range wrongPublicInput {
		wrongPublicInput[i] = new(big.Int).SetUint64(999)
	}

	// call the contract should fail
	ok, err = ev.verifierContract.Verify(nil, proofBytes, wrongPublicInput)
	if err != nil {
		return fmt.Errorf("call verifier with wrong input should have failed")
	}
	if ok {
		return fmt.Errorf("call verifier with wrong input should have returned false")
	}
	return nil
}
