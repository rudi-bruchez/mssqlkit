module github.com/rudi-bruchez/mssqlkit/ddlmatrix

go 1.26.5

// PUBLICATION ORDER MATTERS HERE.
// The replace below is a DEVELOPMENT aid only: `replace` applies to the main
// module and is ignored by consumers, so a downstream `go get .../ddlmatrix`
// would try to fetch platform v0.0.0 and fail. Before publishing ddlmatrix:
//   1. tag platform/vX.Y.Z
//   2. bump the require below to that version
//   3. delete the replace
require github.com/rudi-bruchez/mssqlkit/platform v0.0.0

replace github.com/rudi-bruchez/mssqlkit/platform => ../platform
