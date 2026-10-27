#!/usr/bin/env python3
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class WorkshopPublishDataTest(unittest.TestCase):
    def test_appends_runs_and_manifest(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            snap = ROOT / "testdata" / "workshop-fixtures"
            subprocess.check_call(
                [
                    "python3",
                    str(ROOT / "scripts" / "workshop_publish_data.py"),
                    "--snapshot",
                    str(snap),
                    "--data-dir",
                    str(data),
                    "--retain-snapshots",
                    "10",
                ]
            )
            self.assertTrue((data / "latest.json").exists())
            self.assertTrue((data / "manifest.json").exists())
            self.assertTrue((data / "metrics" / "runs.jsonl").exists())
            self.assertTrue((data / "snapshots" / "fixture-001" / "latest.json").exists())
            row = json.loads((data / "metrics" / "runs.jsonl").read_text(encoding="utf-8").splitlines()[0])
            self.assertEqual(row["run"]["run_id"], "fixture-001")
            manifest = json.loads((data / "manifest.json").read_text(encoding="utf-8"))
            self.assertEqual(manifest["latest_run_id"], "fixture-001")


if __name__ == "__main__":
    unittest.main()
