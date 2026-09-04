package main

import "fmt"

func main() {
	// Thin alias: `tide-sim` forwards to the FleetSim built in Phase 8.
	// Kept as a separate binary name per Architecture §2.12 from day one
	// so deployments/scripts never need renaming later.
	fmt.Println("tide-sim: stub — use `tide simulate` (full simulator is Phase 8 T080-T082)")
}
