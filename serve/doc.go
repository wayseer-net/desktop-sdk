// Package serve runs a module in its own process, for the app to reach over gRPC (Tier 2).
// A module's main is one line:
//
//	func main() { serve.Main(mymodule.New) }
package serve
