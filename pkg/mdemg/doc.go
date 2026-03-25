// Package mdemg provides public API types for external consumers.
//
// This package exports the core request/response types used by the MDEMG API,
// enabling external Go projects to import typed structures for API integration
// without depending on internal packages.
//
// GAP-09: All Go code was previously under internal/, making it impossible
// for external consumers to import types. This package provides the public
// contract surface.
//
// Usage:
//
//	import "mdemg/pkg/mdemg"
//
//	req := mdemg.RetrieveRequest{
//	    SpaceID:   "my-space",
//	    QueryText: "how does retrieval work?",
//	    TopK:      10,
//	}
package mdemg
