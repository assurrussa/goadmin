# Tiptap 3 editor migration

This candidate moves the embedded editor from Tiptap 2.27.3 to a coherent,
exactly pinned 3.32.0 graph. It addresses
[GHSA-cp6q-959q-f8rh](https://github.com/advisories/GHSA-cp6q-959q-f8rh),
whose patched floor is 3.30.4. Upstream references checked on 2026-10-19:
[official v2-to-v3 guide](https://tiptap.dev/docs/guides/upgrade-tiptap-v2),
[patched release](https://github.com/ueberdosis/tiptap/releases/tag/v3.30.4),
and the published 3.32.0 package source/type declarations.

## Preserved editor behavior

The exported RichTextEditor props, emits, media picker callback, initialization
allowlists and invalid-content callback retain their signatures. Existing node
names and CMS payload attributes remain unchanged. There is no storage migration.
The custom Video command and CMS nodes/mark continue to use the supported
Node/Mark/mergeAttributes APIs. JSON round-trip is tested for every media kind.

StarterKit 3 includes Link and Underline: both are disabled there and retain
their existing standalone configuration and allowlist handling. Its new
ListKeymap and TrailingNode extensions are disabled to preserve prior keyboard
and document behavior; no trailing paragraph is silently appended. Table uses
the new named export. The single controlled setContent call uses the v3 options
object with explicit emitUpdate:false and errorOnInvalidContent:true, preserving
feedback prevention and invalid-content reporting. Existing table resizing,
heading levels, image base64 policy, link click policy and task nesting remain.
Table/task options and character/word counters match v2. CharacterCount adds
autoTrim, but its unchanged null limit makes trimming inactive. Link v3 tightens
its default autolinking for bare IPs and single-word hostnames; explicit links
remain supported. This upstream safety behavior is retained.

## Host extension compatibility

This is an upstream major upgrade. Hosts that build their own frontend alongside
GoAdmin must align their Tiptap dependencies and custom extensions with v3;
loading a second v2 core is unsupported. The Go host facade and public rich-text
form contract are unchanged. Go-only consumers receive the rebuilt embedded
assets and need no npm installation.

Upstream API changes that custom hosts may need to adapt include setContent's
(content, options) signature, setContent/clearContent emitting updates by default,
history renamed to undoRedo, table named exports, consolidated list/utility
packages, Vue menu imports under /menus and Floating UI replacing Tippy, and
NodeView getPos potentially returning undefined. GoAdmin uses no custom NodeView,
menu, collaboration, text-style or lowlight implementation; its existing
compatibility package dependencies remain on the same v3 version. New optional
image resize behavior stays disabled. The v3 image schema adds nullable width
and height and Link adds nullable title, so normalized JSON may include those
extra null attributes; existing content is retained. Hosts comparing serialized
JSON bytes should compare the document semantically instead.

The patched helper may retain __proto__ as a harmless own data property; the
regression requires an unchanged prototype and no inherited executable DOM
attributes. It does not promise to sanitize arbitrary explicit event handlers
supplied by a host extension. Host-defined attributes still require validation.

## Validation and release boundary

The candidate includes the complete resource gate, Go race/coverage and static
checks, typed-host graph, manifest/assets validation, a public-dependency consumer
probe and before/after npm audit. Exact command outcomes and source/assets
identities are recorded in the draft PR. Existing SSO/backend acceptance evidence
from the merged source remains applicable because those paths are unchanged;
SSO browser and CA workflows are not repeated. CI remains manual-only.

v0.15.0 publication remains paused. This migration does not merge, tag, release,
deploy or change NotifyHub PR37; its actual released pin is a later step.
