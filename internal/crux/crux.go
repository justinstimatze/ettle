// Package crux is the negotiation seam: when the detector surfaces a CONTESTED
// tangle (a decision-rights conflict or a team-wide divergence — the genuine
// values/priority choices), it routes to a Resolver instead of the mesh quietly
// deciding for the humans.
//
// Two resolvers ship:
//
//   - Gemot — routes to a real gemot deliberation (positions → crux → binding
//     compromise + reputation). Production deployments must reach gemot over
//     TLS with auth: the crux is the most sensitive payload on the wire.
//   - Inline — the infra-free fallback: it frames the tangle as a clean either/or
//     for the humans without any external service, so the PoC runs with nothing
//     installed. The humans still decide; nothing is auto-resolved.
package crux

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/justinstimatze/ettle/internal/ettlemesh"
	"github.com/justinstimatze/ettle/internal/gemotclient"
)

// Resolution is what comes back for a contested tangle: the crux (the real point
// of contention) and, when available, a proposed binding compromise. Source
// names which resolver produced it.
type Resolution struct {
	Source      string  // "gemot" | "inline"
	Crux        string  // the contested claim, sharpened
	Controversy float64 // [0,1] when gemot scored it; 0 for inline
	Proposal    string  // a binding compromise, when one was produced
	Branches    []string
}

// Resolver turns a contested tangle into a Resolution the humans can act on.
type Resolver interface {
	Resolve(ctx context.Context, k ettlemesh.Tangle, atoms []ettlemesh.Atom) (*Resolution, error)
}

// Inline is the zero-dependency resolver. It does not negotiate — it pre-stages
// the tangle as a clean either/or so a human makes the call in seconds. This is
// "friction in the right spot": the choice stays the human's.
type Inline struct{}

func (Inline) Resolve(_ context.Context, k ettlemesh.Tangle, _ []ettlemesh.Atom) (*Resolution, error) {
	return &Resolution{
		Source: "inline",
		Crux:   k.About,
		Branches: []string{
			fmt.Sprintf("%s as %s frames it", k.About, firstParty(k.Parties)),
			fmt.Sprintf("%s as the other party frames it", k.About),
		},
	}, nil
}

// Gemot routes contested tangles through a real gemot deliberation. Reach gemot
// over TLS with auth in any multi-machine deployment (see package doc): Token is
// the gemot bearer key, required off localhost. InsecureLocal opts into gemot's
// anonymous localhost sandbox without a token (dev only).
type Gemot struct {
	URL           string
	Token         string
	InsecureLocal bool
	Timeout       time.Duration
}

func (g Gemot) Resolve(ctx context.Context, k ettlemesh.Tangle, atoms []ettlemesh.Atom) (*Resolution, error) {
	c, err := gemotclient.Connect(ctx, g.URL, g.Token, g.InsecureLocal)
	if err != nil {
		return nil, fmt.Errorf("crux/gemot: connect: %w", err)
	}
	defer c.Close()

	delibID, err := c.Create(k.About, k.Explanation)
	if err != nil {
		return nil, fmt.Errorf("crux/gemot: create: %w", err)
	}
	// Each party submits a position composed from its own atoms — only the
	// typed stance crosses, never raw notes.
	for _, p := range k.Parties {
		pos := positionFor(p, k, atoms)
		if pos == "" {
			pos = fmt.Sprintf("My stance on %s.", k.About)
		}
		if err := c.SubmitPosition(delibID, p, pos, "", ""); err != nil {
			return nil, fmt.Errorf("crux/gemot: submit %s: %w", p, err)
		}
	}
	if err := c.RunAnalysis(delibID); err != nil {
		return nil, fmt.Errorf("crux/gemot: analyze: %w", err)
	}
	to := g.Timeout
	if to == 0 {
		// gemot's multi-round claim-extraction + crux-scoring genuinely takes
		// minutes on a real deliberation; 90s was too impatient (it cut off
		// mid-analysis in live testing).
		to = 180 * time.Second
	}
	raw, err := c.PollResult(delibID, to)
	if err != nil {
		return nil, fmt.Errorf("crux/gemot: poll: %w", err)
	}

	res := &Resolution{Source: "gemot", Crux: k.About}
	if cruxes := gemotclient.Cruxes(raw); len(cruxes) > 0 {
		top := cruxes[0]
		for _, cx := range cruxes {
			if cx.Controversy > top.Controversy {
				top = cx
			}
		}
		res.Crux = top.Claim
		res.Controversy = top.Controversy
	}
	if comps := gemotclient.Compromises(c.ProposeCompromise(delibID)); len(comps) > 0 {
		res.Proposal = comps[0].Proposal
	}
	return res, nil
}

// positionFor joins a party's atoms that touch the tangle's subject into a short
// stance string for gemot.
func positionFor(party string, k ettlemesh.Tangle, atoms []ettlemesh.Atom) string {
	var parts []string
	for _, a := range atoms {
		if !ettlemesh.SamePerson(a.From, party) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", a.Subject, a.Content))
	}
	return strings.Join(parts, "; ")
}

func firstParty(p []string) string {
	if len(p) > 0 {
		return p[0]
	}
	return "one side"
}

// Contested reports whether a tangle is the kind that should go to a resolver
// rather than be quietly bound: decision-rights conflicts and team-wide
// divergences are genuine values/priority choices the humans should own.
func Contested(k ettlemesh.Tangle) bool {
	return k.Firm() && (k.Kind == ettlemesh.KindDecisionRights || k.Kind == ettlemesh.KindTeamwideDivergence)
}
