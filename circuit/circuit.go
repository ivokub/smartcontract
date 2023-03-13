package circuit

import "github.com/consensys/gnark/frontend"

type Circuit struct {
	X, Y frontend.Variable `gnark:",secret"`
	Z    frontend.Variable `gnark:",public"`
}

func (c *Circuit) Define(api frontend.API) error {
	res := api.Mul(c.X, c.Y)
	api.AssertIsEqual(res, c.Z)
	return nil
}
