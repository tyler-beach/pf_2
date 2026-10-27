#!/usr/bin/env python3
"""Tests for changed-line-aware gofmt checks."""

from __future__ import annotations

import unittest

from check_gofmt_changed import changed_ranges_from_git_diff, unformatted_changed_lines


class ChangedLineParsingTests(unittest.TestCase):
    def test_git_diff_ranges_use_new_file_coordinates(self) -> None:
        diff = """diff --git a/example.go b/example.go
index 1111111..2222222 100644
--- a/example.go
+++ b/example.go
@@ -10,1 +10,3 @@
-old
+first
+second
+third
@@ -25,2 +27,1 @@
-old-a
-old-b
+replacement
"""
        self.assertEqual(
            changed_ranges_from_git_diff(diff),
            [(10, 12), (27, 27)],
        )

    def test_insertions_without_explicit_counts_are_one_line(self) -> None:
        diff = """@@ -5 +5,2 @@
-old
+replacement
+new
"""
        self.assertEqual(changed_ranges_from_git_diff(diff), [(5, 6)])

    def test_gofmt_changes_are_detected_when_they_touch_added_lines(self) -> None:
        output = """diff -u example.go.orig example.go
--- example.go.orig
+++ example.go
@@ -3,7 +3,7 @@
 func example() {
-    value:=1
+    value := 1
     use(value)
 }
"""
        self.assertEqual(unformatted_changed_lines(output, [(4, 4)]), [4])

    def test_preexisting_formatting_debt_elsewhere_does_not_fail_changed_lines(self) -> None:
        output = """diff -u example.go.orig example.go
--- example.go.orig
+++ example.go
@@ -3,7 +3,7 @@
 func oldCode() {
-    value:=1
+    value := 1
     use(value)
 }
@@ -40,7 +40,7 @@
 func newCode() {
-    newValue:=2
+    newValue := 2
     use(newValue)
 }
"""
        self.assertEqual(unformatted_changed_lines(output, [(41, 41)]), [41])
        self.assertEqual(unformatted_changed_lines(output, [(7, 7)]), [])

    def test_empty_or_deletion_only_diff_has_no_changed_lines(self) -> None:
        self.assertEqual(changed_ranges_from_git_diff(""), [])
        self.assertEqual(
            changed_ranges_from_git_diff("@@ -8,2 +8,0 @@\n-old-a\n-old-b\n"),
            [],
        )


if __name__ == "__main__":
    unittest.main()
