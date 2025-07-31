package verifier_plonk

type VerifierPlonk struct{}

func DeployVerifierPlonk(args ...any) (any, any, *VerifierPlonk, error) {
	panic("dummy function")
}

func (*VerifierPlonk) Verify(_, _, _ any) (bool, error) {
	panic("dummy function")
}
