package circuit

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/emulated"
	"github.com/consensys/gnark/std/math/emulated/emparams"
)

type Circuit struct {
	X, Y       frontend.Variable                  `gnark:",secret"`
	Z          frontend.Variable                  `gnark:",public"`
	EmuX, EmuY emulated.Element[emparams.BN254Fr] `gnark:",secret"`
	EmuZ       emulated.Element[emparams.BN254Fr] `gnark:",public"`
}

func (c *Circuit) Define(api frontend.API) error {
	// check native multiplication
	res := api.Mul(c.X, c.Y)
	api.AssertIsEqual(res, c.Z)
	// check emulated multiplication
	f, err := emulated.NewField[emparams.BN254Fr](api)
	if err != nil {
		return err
	}
	res2 := f.Mul(&c.EmuX, &c.EmuY)
	f.AssertIsEqual(res2, &c.EmuZ)
	return nil
}
