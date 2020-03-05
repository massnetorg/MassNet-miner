package database_test

import (
	"fmt"

	"massnet.org/mass/config"

	"massnet.org/mass/database"
	"massnet.org/mass/database/memdb"
	"massnet.org/mass/massutil"
)

// This example demonstrates creating a new database and inserting the genesis
// block into it.
func ExampleCreateDB() {
	// Notice in these example imports that the memdb driver is loaded.
	// Ordinarily this would be whatever driver(s) your application
	// requires.

	// Create a database and schedule it to be closed on exit.  This example
	// uses a memory-only database to avoid needing to write anything to
	// the disk.  Typically, you would specify a persistent database driver
	// such as "leveldb" and give it a database name as the second
	// parameter.
	db, err := memdb.NewMemDb()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// Insert the main network genesis block.
	genesis := massutil.NewBlock(config.ChainParams.GenesisBlock)
	err = db.InitByGenesisBlock(genesis)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("New height:", genesis.Height())

	// Output:
	// New height: 0
}

// exampleLoadDB is used in the example to elide the setup code.
func exampleLoadDB() (database.Db, error) {
	db, err := memdb.NewMemDb()
	if err != nil {
		return nil, err
	}

	// Insert the main network genesis block.
	genesis := massutil.NewBlock(config.ChainParams.GenesisBlock)
	err = db.InitByGenesisBlock(genesis)
	if err != nil {
		return nil, err
	}

	return db, err
}

// This example demonstrates querying the database for the most recent best
// block height and hash.
func ExampleDb_newestSha() {
	// Load a database for the purposes of this example and schedule it to
	// be closed on exit.  See the CreateDB example for more details on what
	// this step is doing.
	db, err := exampleLoadDB()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	latestHash, latestHeight, err := db.NewestSha()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Latest hash:", latestHash)
	fmt.Println("Latest height:", latestHeight)

	// Output:
	// Latest hash: ee26300e0f068114a680a772e080507c0f9c0ca4335c382c42b78e2eafbebaa3
	// Latest height: 0
}
