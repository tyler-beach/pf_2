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
            self.assertTrue((data / "snapshots" / "fixture-001" / "sample-journey.json").exists())
            self.assertTrue((data / "sample-journey.json").exists())
            row = json.loads((data / "metrics" / "runs.jsonl").read_text(encoding="utf-8").splitlines()[0])
            self.assertEqual(row["run"]["run_id"], "fixture-001")
            manifest = json.loads((data / "manifest.json").read_text(encoding="utf-8"))
            self.assertEqual(manifest["latest_run_id"], "fixture-001")
            self.assertEqual(
                manifest.get("snapshots"),
                [{"run_id": "fixture-001", "path": "snapshots/fixture-001"}],
            )

    def test_upsert_replaces_same_run_id(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            data = Path(tmp) / "data"
            snap = ROOT / "testdata" / "workshop-fixtures"
            cmd = [
                "python3",
                str(ROOT / "scripts" / "workshop_publish_data.py"),
                "--snapshot",
                str(snap),
                "--data-dir",
                str(data),
                "--retain-snapshots",
                "10",
            ]
            subprocess.check_call(cmd)
            subprocess.check_call(cmd)
            lines = [
                ln
                for ln in (data / "metrics" / "runs.jsonl").read_text(encoding="utf-8").splitlines()
                if ln.strip()
            ]
            self.assertEqual(len(lines), 1)
            row = json.loads(lines[0])
            self.assertEqual(row["run"]["run_id"], "fixture-001")
            self.assertEqual(row["metrics"].get("kpi.confidence.method"), "platt_conformal")


if __name__ == "__main__":
    unittest.main()
