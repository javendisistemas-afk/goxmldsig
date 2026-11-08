package etreeutils

import (
	"sort"
	"strings"

	"github.com/beevik/etree"
)

// TransformExcC14n transforms the passed element into xml-exc-c14n form.
func TransformExcC14n(el *etree.Element, inclusiveNamespacesPrefixList string, comments bool) error {
	return TransformExcC14nWithContext(NewDefaultNSContext(), el, inclusiveNamespacesPrefixList, comments)
}

// TransformExcC14nWithContext is like TransformExcC14n, but lets the caller supply the NSContext
// to resolve prefixes against instead of assuming `el` is the document root. This lets a caller
// canonicalize a sub-element whose namespace prefixes are only declared on an ancestor OUTSIDE
// the subtree being canonicalized (ex.: NSBuildParentContext(el)) — TransformExcC14n alone can
// only resolve prefixes declared on `el` itself or its own descendants.
//
// `declared` (what counts as "already rendered in the canonical output so far") always starts
// empty here, regardless of `ctx`: nothing from an ancestor OUTSIDE the canonicalized subtree is
// actually present in the output, so `el` must still declare, on itself, any namespace it
// visibly utilizes — `ctx` only affects how a prefix RESOLVES, never whether it still needs to be
// declared on `el`.
func TransformExcC14nWithContext(ctx NSContext, el *etree.Element, inclusiveNamespacesPrefixList string, comments bool) error {
	prefixes := strings.Fields(inclusiveNamespacesPrefixList)
	prefixSet := make(map[string]struct{}, len(prefixes))

	for _, prefix := range prefixes {
		prefixSet[prefix] = struct{}{}
	}

	declared := NewDefaultNSContext()
	err := transformExcC14n(ctx, declared, el, prefixSet, comments)
	if err != nil {
		return err
	}

	return nil
}

func transformExcC14n(ctx, declared NSContext, el *etree.Element, inclusiveNamespaces map[string]struct{}, comments bool) error {
	scope, err := ctx.SubContext(el)
	if err != nil {
		return err
	}

	visiblyUtilizedPrefixes := map[string]struct{}{
		el.Space: {},
	}

	filteredAttrs := []etree.Attr{}

	// Filter out all namespace declarations
	for _, attr := range el.Attr {
		switch {
		case attr.Space == xmlnsPrefix:
			if _, ok := inclusiveNamespaces[attr.Key]; ok {
				visiblyUtilizedPrefixes[attr.Key] = struct{}{}
			}

		case attr.Space == defaultPrefix && attr.Key == xmlnsPrefix:
			if _, ok := inclusiveNamespaces[defaultPrefix]; ok {
				visiblyUtilizedPrefixes[defaultPrefix] = struct{}{}
			}

		default:
			if attr.Space != defaultPrefix {
				visiblyUtilizedPrefixes[attr.Space] = struct{}{}
			}

			filteredAttrs = append(filteredAttrs, attr)
		}
	}

	el.Attr = filteredAttrs

	declared = declared.Copy()

	// Declare all visibly utilized prefixes that are in-scope but haven't
	// been declared in the canonicalized form yet. These might have been
	// declared on this element but then filtered out above, or they might
	// have been declared on an ancestor (before canonicalization) which
	// didn't visibly utilize and thus had them removed.
	for prefix := range visiblyUtilizedPrefixes {
		// Skip redundant declarations - they have to already have the same
		// value.
		if declaredNamespace, ok := declared.prefixes[prefix]; ok {
			if value, ok := scope.prefixes[prefix]; ok && declaredNamespace == value {
				continue
			}
		}

		namespace, err := scope.LookupPrefix(prefix)
		if err != nil {
			return err
		}

		el.Attr = append(el.Attr, declared.declare(prefix, namespace))
	}

	sort.Sort(SortedAttrs(el.Attr))

	if !comments {
		c := 0
		for c < len(el.Child) {
			if _, ok := el.Child[c].(*etree.Comment); ok {
				el.RemoveChildAt(c)
			} else {
				c++
			}
		}
	}

	// Transform child elements
	for _, child := range el.ChildElements() {
		err := transformExcC14n(scope, declared, child, inclusiveNamespaces, comments)
		if err != nil {
			return err
		}
	}

	return nil
}
