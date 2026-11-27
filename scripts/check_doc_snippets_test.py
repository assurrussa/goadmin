"""Focused tests for the offline documentation synchronizer."""
import importlib.util
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("snippets", Path(__file__).with_name("check-doc-snippets.py"))
snippets = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(snippets)


class SnippetTests(unittest.TestCase):
    def setUp(self):
        self.excerpt = snippets.Excerpt("docs/queue.md", "example/queue.go", "module", "migrate")
        self.opening = self.excerpt.marker()
        self.closing = self.excerpt.marker(closing=True)
        self.function = "func module() {\n\treturn\n}"
        self.source = f"package main\n\n{self.function}\n\nfunc migrate() {{}}\n"
        self.doc = f"before\n{self.opening}\nstale\n{self.closing}\nafter\n"

    def render(self, doc, source):
        return snippets.rendered_document(doc, source, self.excerpt)

    def test_replaces_only_marked_block(self):
        rendered = self.render(self.doc, self.source)
        self.assertEqual(rendered, self.doc.replace("stale", f"```go\n{self.function}\n```"))

    def test_refresh_is_idempotent(self):
        rendered = self.render(self.doc, self.source)
        self.assertEqual(rendered, self.render(rendered, self.source))

    def test_source_change_produces_drift(self):
        rendered = self.render(self.doc, self.source)
        self.assertNotEqual(rendered, self.render(rendered, self.source.replace("return", "panic(1)")))

    def test_missing_duplicate_and_reversed_markers_fail(self):
        for doc in ("", self.doc + self.opening, self.closing + self.opening):
            with self.subTest(doc=doc), self.assertRaises(ValueError):
                self.render(doc, self.source)

    def test_inserted_helper_is_not_silently_documented(self):
        changed = self.source.replace("func migrate", "func helper() {}\n\nfunc migrate")
        with self.assertRaises(ValueError):
            self.render(self.doc, changed)

    def test_changed_function_boundaries_fail(self):
        for source in ("", self.source + "\n" + self.function, "\nfunc migrate() {}\n" + self.function):
            with self.subTest(source=source), self.assertRaises(ValueError):
                self.render(self.doc, source)


if __name__ == "__main__":
    unittest.main()
