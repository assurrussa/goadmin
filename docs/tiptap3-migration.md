# Tiptap 3 editor migration

This candidate deliberately moves the embedded editor from Tiptap 2.27.3 to a
coherent, exactly pinned 3.32.0 graph as the project's chosen development path.
It is **not required solely to repair GHSA-cp6q-959q-f8rh**: 2.27.3 already
contains the [v2 backport](https://github.com/ueberdosis/tiptap/pull/8301),
released in [2.27.3](https://github.com/ueberdosis/tiptap/pull/8310).
The advisory metadata omitted the v2 patched version; see
[upstream issue #8339](https://github.com/ueberdosis/tiptap/issues/8339).
The earlier claim that the installed v2 helper remained vulnerable was incorrect.
The audit changed from 35 moderate records for that one advisory to 0 records;
this does not prove the baseline lacked the backport or certify the entire graph.

Upstream references checked on 2026-10-19:
[official v2-to-v3 guide](https://tiptap.dev/docs/guides/upgrade-tiptap-v2),
[v3 patched release](https://github.com/ueberdosis/tiptap/releases/tag/v3.30.4),
and the published 3.32.0 package source/type declarations.

## Preserved editor behavior

The exported RichTextEditor props, emits, media picker callback, initialization
allowlists and invalid-content callback retain their signatures. Existing node
names and CMS payload attributes remain unchanged. There is no storage migration.
The custom Video command and CMS nodes/mark continue to use the supported
Node/Mark/mergeAttributes APIs. JSON round-trip is tested for every media kind.

StarterKit 3 includes Link and Underline: both are disabled there and retain
their existing standalone configuration and allowlist handling. Its new
ListKeymap and TrailingNode extensions are disabled to avoid enabling their
additional behavior; no trailing paragraph is silently appended. This does not
freeze all list keyboard behavior at v2: ListItem and nested TaskItem still
install v3 branching Delete/Mod-Delete handlers. At a textblock end before a
branching nested list, both keys hoist that list's items to the parent list,
retaining the child sublists, text, marks, order and task checked states.
Focused regressions cover bullet, ordered and nested task lists and undo/redo.
These v3 semantics are accepted; arbitrary key combinations, custom host
keymaps and platform/browser-native editing are not certified by these tests.
Table uses the new named export. The single controlled setContent call uses the v3 options
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
extra null attributes. TableCell and TableHeader also add nullable align;
HTML serialization omits default colspan="1" and rowspan="1", while preserving
non-default spans. These are normalization differences rather than content
removal. A regression checks table content/spans through normal HTML round-trip.
Hosts comparing serialized JSON bytes should compare the document semantically
instead.

The patched helper may retain __proto__ as a harmless own data property; the
regression requires an unchanged prototype and no inherited executable DOM
attributes. It does not promise to sanitize arbitrary explicit event handlers
supplied by a host extension. Host-defined attributes still require validation.
A representative host Video extension is tested for inherited attributes/rendering, a renamed node, inherited
setVideo command and JSON updates. That case and the arbitrary-map DOMSerializer
regression do not certify all third-party host extensions.

## Known upstream boundaries

The real Delete/Mod-Delete key handlers preserve the tested branching list
content. A separate probe found that v3.32.0's synthetic
`editor.commands.keyboardShortcut('Delete')` loses the hoisted child branch in
that same document (parent and following sibling remain); the actual key event
does not. The equivalent v2.27.3 probe retains the text for both paths. GoAdmin
runtime contains no keyboardShortcut calls and does not expose an editor-instance
prop/callback, so this is not a demonstrated first-party UI loss. Custom hosts
that obtain editor instances or call this upstream command must review those
calls before adopting v3. This migration does not certify that synthetic API or
patch upstream internals. The focused regression invokes the actual ProseMirror
key handlers, not that command helper.

Strict HTML content validation rejects generated table HTML containing tbody
in both v2.27.3 and v3.32.0; normal parsing round-trips the tested valid table.
The strict controlled-content path continues to report invalid content rather
than silently accepting it. Prefer the default JSON contract for table documents.
This is a confirmed pre-existing limitation, not a new v3 regression, and no
validation setting is weakened here.

## Validation and release boundary

The initial candidate passed the complete resource gate, Go race/coverage and static
checks, typed-host graph, manifest/assets validation, a public-dependency consumer
probe and before/after npm audit. Exact command outcomes and source/assets
identities are recorded in the draft PR. Existing SSO/backend acceptance evidence
from the merged source remains applicable because those paths are unchanged;
SSO browser and CA workflows are not repeated. The final follow-up changes only
documentation and compatibility tests; it runs focused tests, lint/format and
type checks. The previous 647-test full UI/make-check, audit0, anonymous-consumer
and canonical-asset evidence is retained for the unchanged runtime/dependency/
asset tree rather than claimed as a rerun of the new test count. CI remains
manual-only.

v0.15.0 publication remains paused. This migration does not merge, tag, release,
deploy or change NotifyHub PR37; its actual released pin is a later step.
