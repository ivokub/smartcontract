package verifier_plonk

type VerifierPlonk struct{}

func DeployVerifierPlonk(args ...any) (any, any, *VerifierPlonk, error) {
	panic("dummy function")
}

func (*VerifierPlonk) VerifyProof(_, _, _, _, _ any) error {
	panic("dummy function")
}
