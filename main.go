package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/big"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/backends"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ivokub/smartcontract/circuit"
	"github.com/ivokub/smartcontract/verifier"
)

const (
	NAME = "RSA.G16"
)

var (
	VKNAME  = NAME + ".vk"
	PKNAME  = NAME + ".pk"
	SOLNAME = NAME + ".sol"
	CCSNAME = NAME + ".ccs"
)

var curve = ecc.BN254

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Println("subcommand 'generate' or 'test'")
	}
	switch args[0] {
	case "generate":
		if err := generateGroth16(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	case "test":
		ev, err := setup()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := run(ev); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	default:
		fmt.Println("unknown subcommand. valid commands 'generate' and 'test'")
	}
	fmt.Println("OK!")
}

func generateGroth16() error {
	var circuit circuit.Circuit

	ccs, err := frontend.Compile(curve.ScalarField(), r1cs.NewBuilder, &circuit)
	if err != nil {
		return err
	}
	pk, vk, err := groth16.Setup(ccs)
	if err != nil {
		return err
	}

	fccs, err := os.Create(CCSNAME)
	if err != nil {
		return err
	}
	defer fccs.Close()
	_, err = ccs.WriteTo(fccs)
	if err != nil {
		return err
	}

	fvk, err := os.Create(VKNAME)
	if err != nil {
		return err
	}
	defer fvk.Close()
	_, err = vk.WriteRawTo(fvk)
	if err != nil {
		return err
	}

	fpk, err := os.Create(PKNAME)
	if err != nil {
		return err
	}
	defer fpk.Close()
	_, err = pk.WriteRawTo(fpk)
	if err != nil {
		return err
	}

	fsol, err := os.Create(SOLNAME)
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

type ethVerifier struct {
	// backend
	backend *backends.SimulatedBackend

	// verifier contract
	verifierContract *verifier.Verifier

	// groth16 gnark objects
	vk      groth16.VerifyingKey
	pk      groth16.ProvingKey
	circuit circuit.Circuit
	r1cs    constraint.ConstraintSystem
}

func setup() (*ethVerifier, error) {
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
	caddr, _, v, err := verifier.DeployVerifier(auth, newbackend)
	if err != nil {
		return nil, fmt.Errorf("new verifier: %w", err)
	}
	newbackend.Commit()
	fmt.Printf("deployed contract at %s\n", caddr)

	fccs, err := os.Open(CCSNAME)
	if err != nil {
		return nil, fmt.Errorf("open ccs: %w", err)
	}
	defer fccs.Close()
	ccs := groth16.NewCS(curve)
	_, err = ccs.ReadFrom(fccs)
	if err != nil {
		return nil, fmt.Errorf("read ccs: %w", err)
	}
	fpk, err := os.Open(PKNAME)
	if err != nil {
		return nil, fmt.Errorf("open pk: %w", err)
	}
	defer fpk.Close()
	pk := groth16.NewProvingKey(curve)
	_, err = pk.ReadFrom(fpk)
	if err != nil {
		return nil, fmt.Errorf("read pk: %w", err)
	}
	fvk, err := os.Open(VKNAME)
	if err != nil {
		return nil, fmt.Errorf("open vk: %w", err)
	}
	defer fvk.Close()
	vk := groth16.NewVerifyingKey(curve)
	_, err = vk.ReadFrom(fvk)
	if err != nil {
		return nil, fmt.Errorf("read vk: %w", err)
	}
	return &ethVerifier{
		backend:          newbackend,
		verifierContract: v,
		vk:               vk,
		pk:               pk,
		circuit:          circuit.Circuit{},
		r1cs:             ccs,
	}, nil
}

func run(ev *ethVerifier) error {
	n := 15
	p, q := 3, 5
	assignment := circuit.Circuit{
		X: p,
		Y: q,
		Z: n,
	}

	// witness creation
	witness, err := frontend.NewWitness(&assignment, curve.ScalarField())
	if err != nil {
		return fmt.Errorf("new witness: %w", err)
	}

	// prove
	proof, err := groth16.Prove(ev.r1cs, ev.pk, witness)
	if err != nil {
		return fmt.Errorf("prove: %w", err)
	}

	// ensure gnark (Go) code verifies it
	publicWitness, err := witness.Public()
	if err != nil {
		return fmt.Errorf("new public witness: %w", err)
	}
	if err = groth16.Verify(proof, ev.vk, publicWitness); err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	// get proof bytes
	const fpSize = 4 * 8
	var buf bytes.Buffer
	proof.WriteRawTo(&buf)
	proofBytes := buf.Bytes()

	// solidity contract inputs
	var (
		a     [2]*big.Int
		b     [2][2]*big.Int
		c     [2]*big.Int
		input [1]*big.Int
	)

	// proof.Ar, proof.Bs, proof.Krs
	a[0] = new(big.Int).SetBytes(proofBytes[fpSize*0 : fpSize*1])
	a[1] = new(big.Int).SetBytes(proofBytes[fpSize*1 : fpSize*2])
	b[0][0] = new(big.Int).SetBytes(proofBytes[fpSize*2 : fpSize*3])
	b[0][1] = new(big.Int).SetBytes(proofBytes[fpSize*3 : fpSize*4])
	b[1][0] = new(big.Int).SetBytes(proofBytes[fpSize*4 : fpSize*5])
	b[1][1] = new(big.Int).SetBytes(proofBytes[fpSize*5 : fpSize*6])
	c[0] = new(big.Int).SetBytes(proofBytes[fpSize*6 : fpSize*7])
	c[1] = new(big.Int).SetBytes(proofBytes[fpSize*7 : fpSize*8])

	// public witness
	input[0] = new(big.Int).SetUint64(uint64(n))

	// call the contract
	res, err := ev.verifierContract.VerifyProof(nil, a, b, c, input)
	if err != nil {
		return fmt.Errorf("calling verifier: %w", err)
	}
	if !res {
		return fmt.Errorf("should have succeeded")
	}

	// (wrong) public witness
	input[0] = new(big.Int).SetUint64(999)

	// call the contract should fail
	res, err = ev.verifierContract.VerifyProof(nil, a, b, c, input)
	if err != nil {
		return fmt.Errorf("call verifier wrong input: %w", err)
	}
	if res {
		return fmt.Errorf("should have failed")
	}
	return nil
}
