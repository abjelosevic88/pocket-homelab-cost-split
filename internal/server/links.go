package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-cost-split/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core): another
// Pocket app may act here for a person with a pcx_ token, only within these
// scopes. Every route not listed in linkRoutes stays closed to it, it sees
// exactly the groups the person sees, and it may change or delete only the
// expenses it added itself (expenses.source = its slug).

var linkOffers = []core.Offer{
	{Scope: "groups:read", Name: "See your groups and expenses", Description: "List the groups you're in, their members, categories and expenses."},
	{Scope: "expenses:write", Name: "Add expenses", Description: "Add expenses to groups you're in (for example a shopping trip), and change or delete only the ones that app added."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/groups$`), "groups:read"},
	{"GET", regexp.MustCompile(`^/api/v1/groups/\d+/?$`), "groups:read"},
	{"GET", regexp.MustCompile(`^/api/v1/groups/\d+/expenses$`), "groups:read"},
	{"GET", regexp.MustCompile(`^/api/v1/groups/\d+/expenses/\d+$`), "groups:read"},
	{"GET", regexp.MustCompile(`^/api/v1/categories$`), "groups:read"},
	{"POST", regexp.MustCompile(`^/api/v1/groups/\d+/expenses$`), "expenses:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/groups/\d+/expenses/\d+$`), "expenses:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// principal is how a request came in; only link requests carry one.
type principal struct {
	Kind     string // link
	LinkApp  string // acting app slug, e.g. "kitchen"
	LinkName string // acting app name, e.g. "Pocket Kitchen"
	Scopes   []string
}

const principalKey ctxKey = 3

func principalFrom(ctx context.Context) *principal {
	p, _ := ctx.Value(principalKey).(*principal)
	return p
}

// linkApp returns the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	if p := principalFrom(ctx); p != nil && p.Kind == "link" {
		return p.LinkApp
	}
	return ""
}

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkPrincipal validates a pcx_ token, checks the route against its scopes
// and returns who the app acts for.
func (s *Server) linkPrincipal(ctx context.Context, tok string, r *http.Request) (*principal, string, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", "", err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", "", http.ErrNoCookie
	}
	p := &principal{Kind: "link", LinkApp: in.App.Slug, LinkName: in.App.Name, Scopes: strings.Fields(in.Scope)}
	if p.LinkName == "" {
		p.LinkName = in.App.Slug
	}
	if !linkAllows(p.Scopes, r) {
		return nil, "", "", errLinkScope
	}
	return p, strings.ToLower(in.User.Email), in.User.Name, nil
}

// linkOwns reports whether the request may change or delete the expense: a
// person always may (group rules apply as before), a linked app only when it
// added the expense itself.
func linkOwns(ctx context.Context, e *store.Expense) bool {
	app := linkApp(ctx)
	return app == "" || e.Source == app
}

// viaLink is " · via Pocket Kitchen" for the activity feed when a linked app
// made the change, empty otherwise.
func viaLink(ctx context.Context) string {
	if p := principalFrom(ctx); p != nil && p.Kind == "link" {
		return " · via " + p.LinkName
	}
	return ""
}
