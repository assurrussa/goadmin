"""Focused tests for the offline documentation synchronizer."""
import importlib.util
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("snippets", Path(__file__).with_name("check-doc-snippets.py"))
snippets = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(snippets)


class SnippetTests(unittest.TestCase):
    def setUp(self):
        self.source = snippets.SOURCE_START + "\n\treturn nil, nil\n}\n" + snippets.SOURCE_END + "ctx) {}\n"
        self.doc = "before\n" + snippets.START + "\nstale\n" + snippets.END + "\nafter\n"

    def test_replaces_only_marked_block(self):
        rendered = snippets.rendered_document(self.doc, self.source)
        self.assertTrue(rendered.startswith("before\n" + snippets.START))
        self.assertTrue(rendered.endswith(snippets.END + "\nafter\n"))
        self.assertIn("```go\n" + snippets.SOURCE_START, rendered)
        self.assertNotIn("migrateProducts", rendered)
        self.assertNotIn("stale", rendered)

    def test_refresh_is_idempotent(self):
        rendered = snippets.rendered_document(self.doc, self.source)
        self.assertEqual(rendered, snippets.rendered_document(rendered, self.source))

    def test_source_change_produces_drift(self):
        rendered = snippets.rendered_document(self.doc, self.source)
        self.assertNotEqual(rendered, snippets.rendered_document(rendered, self.source.replace("nil, nil", "module, nil")))

    def test_missing_duplicate_and_reversed_markers_fail(self):
        for doc in ("", self.doc + snippets.START, snippets.END + snippets.START):
            with self.subTest(doc=doc), self.assertRaises(ValueError):
                snippets.rendered_document(doc, self.source)

    def test_inserted_helper_is_not_silently_documented(self):
        changed = self.source.replace(snippets.SOURCE_END, "\nfunc helper() {}\n" + snippets.SOURCE_END)
        with self.assertRaises(ValueError):
            snippets.source_excerpt(changed)

    def test_changed_function_boundaries_fail(self):
        for source in ("", self.source + snippets.SOURCE_START, snippets.SOURCE_END + snippets.SOURCE_START):
            with self.subTest(source=source), self.assertRaises(ValueError):
                snippets.source_excerpt(source)


if __name__ == "__main__":
    unittest.main()
