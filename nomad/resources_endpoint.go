package nomad

import (
	"fmt"
	"github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

// truncateLimit is the maximum number of matches that will be returned for a
// prefix for a specific context
const truncateLimit = 20

// Resource endpoint is used to lookup matches for a given prefix and context
type Resources struct {
	srv *Server
}

// getMatches extracts matches for an iterator, and returns a list of ids for
// these matches.
func getMatches(iter memdb.ResultIterator) ([]string, bool) {
	var matches []string
	isTruncated := false

	for i := 0; i < truncateLimit; i++ {
		raw := iter.Next()
		if raw == nil {
			break
		}

		var id string
		switch raw.(type) {
		case *structs.Job:
			id = raw.(*structs.Job).ID
		case *structs.Evaluation:
			id = raw.(*structs.Evaluation).ID
		case *structs.Allocation:
			id = raw.(*structs.Allocation).ID
		case *structs.Node:
			id = raw.(*structs.Node).ID
		default:
			continue
		}

		matches = append(matches, id)
	}

	if iter.Next() != nil {
		isTruncated = true
	}

	return matches, isTruncated
}

// getResourceIter takes a context and returns a memdb iterator specific to
// that context
func getResourceIter(context, prefix string, ws memdb.WatchSet, state *state.StateStore) (memdb.ResultIterator, error) {
	switch context {
	case "jobs":
		return state.JobsByIDPrefix(ws, prefix)
	case "evals":
		return state.EvalsByIDPrefix(ws, prefix)
	case "allocs":
		return state.AllocsByIDPrefix(ws, prefix)
	case "nodes":
		return state.NodesByIDPrefix(ws, prefix)
	default:
		return nil, fmt.Errorf("invalid context")
	}
}

// List is used to list the resouces registered in the system that matches the
// given prefix. Resources are jobs, evaluations, allocations, and/or nodes.
func (r *Resources) List(args *structs.ResourcesRequest,
	reply *structs.ResourcesResponse) error {
	reply.Matches = make(map[string][]string)
	reply.Truncations = make(map[string]bool)

	// Setup the blocking query
	opts := blockingOptions{
		queryMeta: &reply.QueryMeta,
		queryOpts: &structs.QueryOptions{},
		run: func(ws memdb.WatchSet, state *state.StateStore) error {

			iters := make(map[string]memdb.ResultIterator)

			if args.Context != "" {
				iter, err := getResourceIter(args.Context, args.Prefix, ws, state)
				if err != nil {
					return err
				}
				iters[args.Context] = iter
			} else {
				for _, e := range []string{"allocs", "nodes", "jobs", "evals"} {
					iter, err := getResourceIter(e, args.Prefix, ws, state)
					if err != nil {
						return err
					}
					iters[e] = iter
				}
			}

			// Return matches for the given prefix
			for k, v := range iters {
				res, isTrunc := getMatches(v)
				reply.Matches[k] = res
				reply.Truncations[k] = isTrunc
			}

			// Set the index for the context. If the context has been specified, it
			// is the only non-empty match set, and the index is set for it.
			// If the context was not specified, we set the index of the first
			// non-empty match set.
			for k, v := range reply.Matches {
				if len(v) != 0 {
					index, err := state.Index(k)
					if err != nil {
						return err
					}
					reply.Index = index
					break
				}
			}

			r.srv.setQueryMeta(&reply.QueryMeta)
			return nil
		}}
	return r.srv.blockingRPC(&opts)
}
