package verifier_groth16

type Verifier struct{}

func DeployVerifier(args ...any) (any, any, *Verifier, error) {
	panic("dummy function")
}

func (*Verifier) VerifyProof(_, _, _, _, _ any) error {
	panic("dummy function")
}
