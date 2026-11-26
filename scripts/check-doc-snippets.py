#!/usr/bin/env python3
"""Check selected Markdown Go excerpts against their runnable source (offline).

Use --write to refresh excerpts after reviewing source changes. This does not
compile Go or claim browser/database acceptance; run the source's existing tests.
"""
import argparse
from pathlib import Path
from typing import NamedTuple

ROOT = Path(__file__).resolve().parents[1]


class Excerpt(NamedTuple):
    document: str
    source: str
    function: str
    next_function: str

    def marker(self, closing=False):
        prefix = "/source" if closing else "source"
        return f"<!-- {prefix}: {self.source}#{self.function} -->"


EXCERPTS = (
    Excerpt("docs/first-crud.md", "examples/starter/products.go", "productsModule", "migrateProducts"),
    Excerpt("docs/crud.md", "examples/starter/requests.go", "requestsModule", "authorizeRequest"),
)


def source_excerpt(source, excerpt):
    start_marker = f"\nfunc {excerpt.function}("
    end_marker = f"\nfunc {excerpt.next_function}("
    if source.count(start_marker) != 1 or source.count(end_marker) != 1:
        raise ValueError(f"{excerpt.source}: review function boundaries for {excerpt.function}")
    start = source.index(start_marker) + 1
    end = source.index(end_marker)
    if end <= start:
        raise ValueError("Source function boundaries are out of order")
    body = source[start:end].rstrip()
    if sum(line.startswith("func ") for line in body.splitlines()) != 1:
        raise ValueError("A function was inserted inside the excerpt boundaries; review the selector")
    return body


def rendered_document(document, source, excerpt):
    opening, closing = excerpt.marker(), excerpt.marker(closing=True)
    if document.count(opening) != 1 or document.count(closing) != 1:
        raise ValueError(f"{excerpt.document}: expected exactly one marker pair for {excerpt.function}")
    start = document.index(opening) + len(opening)
    end = document.index(closing)
    if end <= start:
        raise ValueError("Documentation markers are out of order")
    return document[:start] + "\n```go\n" + source_excerpt(source, excerpt) + "\n```\n" + document[end:]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="refresh the reviewed source excerpts")
    args = parser.parse_args()
    for excerpt in EXCERPTS:
        document_path = ROOT / excerpt.document
        document = document_path.read_text()
        expected = rendered_document(document, (ROOT / excerpt.source).read_text(), excerpt)
        if document != expected:
            if not args.write:
                parser.exit(1, f"{excerpt.document}: excerpt drift; review source, then run python3 scripts/check-doc-snippets.py --write\n")
            document_path.write_text(expected)
        print(f"PASS: {excerpt.document} matches {excerpt.source}#{excerpt.function}")


if __name__ == "__main__":
    main()
