#!/usr/bin/env python3
"""Check selected Markdown Go excerpts against their runnable source (offline).

Use --write to refresh excerpts after reviewing source changes. This does not
compile Go or claim browser/database acceptance; run the source's existing tests.
"""
import argparse
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
START = "<!-- source: examples/starter/products.go#productsModule -->"
END = "<!-- /source: examples/starter/products.go#productsModule -->"
SOURCE_START = "func productsModule(db *sql.DB) (host.Module, error) {"
SOURCE_END = "\nfunc migrateProducts("


def source_excerpt(source):
    if source.count(SOURCE_START) != 1 or source.count(SOURCE_END) != 1:
        raise ValueError("Products function boundaries changed; review the excerpt selector")
    start = source.index(SOURCE_START)
    end = source.index(SOURCE_END)
    if end <= start:
        raise ValueError("Products function boundaries are out of order")
    excerpt = source[start:end].rstrip()
    if sum(line.startswith("func ") for line in excerpt.splitlines()) != 1:
        raise ValueError("A function was inserted inside the excerpt boundaries; review the selector")
    return excerpt


def rendered_document(document, source):
    if document.count(START) != 1 or document.count(END) != 1:
        raise ValueError("Documentation must contain exactly one matching marker pair")
    start = document.index(START) + len(START)
    end = document.index(END)
    if end <= start:
        raise ValueError("Documentation markers are out of order")
    return document[:start] + "\n```go\n" + source_excerpt(source) + "\n```\n" + document[end:]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="refresh the reviewed source excerpt")
    args = parser.parse_args()
    document_path = ROOT / "docs/first-crud.md"
    document = document_path.read_text()
    source = (ROOT / "examples/starter/products.go").read_text()
    expected = rendered_document(document, source)
    if document != expected:
        if not args.write:
            parser.exit(1, "CRUD excerpt drift: review source, then run python3 scripts/check-doc-snippets.py --write\n")
        document_path.write_text(expected)
        print("Updated first-crud.md from the existing Products source")
    else:
        print("PASS: CRUD Markdown excerpt matches the existing Products source")


if __name__ == "__main__":
    main()
