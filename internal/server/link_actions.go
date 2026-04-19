package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mkende/golink-url-shortener/internal/db"
	"github.com/mkende/golink-url-shortener/internal/links"
	"github.com/mkende/golink-url-shortener/internal/redirect"
)

// createLinkParams holds the already-parsed, normalised inputs for creating a link.
// Target must be trimmed; AliasTarget must be lower-cased.
type createLinkParams struct {
	Name        string
	Target      string
	LinkType    db.LinkType
	AliasTarget string
	RequireAuth bool
}

// createLinkErrorKind classifies the failure mode of doCreateLink so callers
// can map it to an appropriate HTTP status code.
type createLinkErrorKind int

const (
	// createErrValidation is a user-visible input error; callers should use 400.
	createErrValidation createLinkErrorKind = iota
	// createErrConflict means a link with that name already exists; use 409.
	createErrConflict
	// createErrInternal is a DB or other server failure; use 500.
	createErrInternal
)

// createLinkError is returned by doCreateLink to describe the failure.
type createLinkError struct {
	Kind    createLinkErrorKind
	Message string
}

func (e *createLinkError) Error() string { return e.Message }

// doCreateLink validates params and creates a new link.
//
// For alias links the alias target is resolved one level of indirection
// (so pointing at an alias of an alias just creates an alias of the canonical
// link) and the per-link alias limit is enforced.
//
// Returns the created link on success, or a *createLinkError describing the
// failure.  Callers should type-assert the returned error to *createLinkError
// and use the Kind field to choose an HTTP status code.
func (s *Server) doCreateLink(r *http.Request, params createLinkParams, ownerEmail string) (*db.Link, *createLinkError) {
	if err := links.ValidateName(params.Name); err != nil {
		return nil, &createLinkError{Kind: createErrValidation, Message: err.Error()}
	}

	if params.LinkType == db.LinkTypeAlias {
		if params.AliasTarget == "" {
			return nil, &createLinkError{Kind: createErrValidation, Message: "alias target cannot be empty"}
		}
		resolved, err := s.resolveAliasTarget(r, params.AliasTarget, "")
		if err != nil {
			return nil, &createLinkError{Kind: createErrValidation, Message: err.Error()}
		}
		params.AliasTarget = resolved

		count, err := s.links.CountAliases(r.Context(), params.AliasTarget)
		if err != nil {
			return nil, &createLinkError{Kind: createErrInternal, Message: err.Error()}
		}
		if count >= s.cfg.MaxAliasesPerLink {
			return nil, &createLinkError{
				Kind:    createErrValidation,
				Message: fmt.Sprintf("alias limit reached: a link may have at most %d aliases", s.cfg.MaxAliasesPerLink),
			}
		}
	} else {
		if params.LinkType == db.LinkTypeAdvanced && !s.cfg.AdvancedLinksAllowed() {
			return nil, &createLinkError{Kind: createErrValidation, Message: "advanced links are disabled by the server configuration"}
		}
		if err := links.ValidateTarget(params.Target); err != nil {
			return nil, &createLinkError{Kind: createErrValidation, Message: err.Error()}
		}
		if params.LinkType == db.LinkTypeAdvanced {
			if err := redirect.CheckTemplateTargetDomain(params.Target, s.cfg.DomainsForAdvancedLinks); err != nil {
				return nil, &createLinkError{Kind: createErrValidation, Message: err.Error()}
			}
		}
	}

	link, err := s.links.Create(r.Context(), params.Name, params.Target, ownerEmail,
		params.LinkType, params.AliasTarget, params.RequireAuth)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "unique") {
			return nil, &createLinkError{Kind: createErrConflict, Message: "link name already exists"}
		}
		return nil, &createLinkError{Kind: createErrInternal, Message: err.Error()}
	}
	return link, nil
}
