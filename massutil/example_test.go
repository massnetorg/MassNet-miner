package massutil_test

import (
	"fmt"
	"math"

	"massnet.org/mass/massutil"
)

func ExampleAmount() {

	a := massutil.ZeroAmount()
	fmt.Println("Zero Maxwell:", a)

	a, _ = massutil.NewAmountFromUint(100000000)
	fmt.Println("100,000,000 Maxwells:", a)

	a, _ = massutil.NewAmountFromUint(100000)
	fmt.Println("100,000 Maxwells:", a)
	// Output:
	// Zero Maxwell: 0 MASS
	// 100,000,000 Maxwells: 1 MASS
	// 100,000 Maxwells: 0.001 MASS
}

func ExampleNewAmountFromMass() {
	amountOne, err := massutil.NewAmountFromMass(1)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountOne) //Output 1

	amountFraction, err := massutil.NewAmountFromMass(0.01234567)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountFraction) //Output 2

	amountZero, err := massutil.NewAmountFromMass(0)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountZero) //Output 3

	amountNaN, err := massutil.NewAmountFromMass(math.NaN())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(amountNaN) //Output 4

	// Output: 1 MASS
	// 0.01234567 MASS
	// 0 MASS
	// invalid float
}

func ExampleAmount_unitConversions() {
	amount, _ := massutil.NewAmountFromUint(44433322211100)

	fmt.Println("Maxwell to kMASS:", amount.Format(massutil.AmountKiloMASS))
	fmt.Println("Maxwell to MASS:", amount)
	fmt.Println("Maxwell to MilliMASS:", amount.Format(massutil.AmountMilliMASS))
	fmt.Println("Maxwell to MicroMASS:", amount.Format(massutil.AmountMicroMASS))
	fmt.Println("Maxwell to Maxwell:", amount.Format(massutil.AmountMaxwell))

	// Output:
	// Maxwell to kMASS: 444.333222111 kMASS
	// Maxwell to MASS: 444333.222111 MASS
	// Maxwell to MilliMASS: 444333222.111 mMASS
	// Maxwell to MicroMASS: 444333222111 μMASS
	// Maxwell to Maxwell: 44433322211100 Maxwell
}
