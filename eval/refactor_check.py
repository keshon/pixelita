"""Run reproducible CLI acceptance against generate_corpus.py outputs."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser()
parser.add_argument("binary", type=Path)
parser.add_argument("corpus", type=Path)
parser.add_argument("report", type=Path)
args = parser.parse_args()
binary = str(args.binary.resolve())
records = []

def run(*argv, expected=0):
    started = time.perf_counter()
    proc = subprocess.run([binary, *map(str, argv), "--json"], capture_output=True, text=True)
    result = json.loads(proc.stdout)
    records.append({"args": list(map(str, argv)), "exit": proc.returncode,
                    "seconds": time.perf_counter() - started, "report": result})
    if proc.returncode != expected:
        raise AssertionError((argv, proc.returncode, result, proc.stderr))
    return result

with tempfile.TemporaryDirectory(prefix="pixelita-acceptance-") as directory:
    out = Path(directory)
    files = [args.corpus / name for name in ("flat-rgba.png", "photo-rgb.png", "baseline.jpg", "progressive.jpg")]
    run("capabilities")
    run("inspect", *files)
    preview = run("optimize", *files, "--out-dir", out / "same")
    applied = run("optimize", *files, "--out-dir", out / "same", "--apply")
    assert [i["status"] for i in preview["items"]] == ["would" if i["status"] == "done" else i["status"] for i in applied["items"]]
    for item in applied["items"]:
        if item["status"] == "done":
            extra = ["--min-psnr", "200"] if item["path"].endswith(".jpg") else []
            run("compare", item["path"], item["output"], *extra)
    run("optimize", files[1], "--format", "auto", "--out-dir", out / "auto")
    variants = run("optimize", files[1], "--format", "webp", "--widths", "160,320", "--out-dir", out / "variants", "--apply")
    for item in variants["items"]:
        assert item["status"] == "done"
        run("compare", item["path"], item["output"], "--worst", "3", "--levels")
    run("view", files[1], "--crop", "0,0,32,32 32,32,32,32", "--stats", "--dry-run")
    run("optimize", files[0], "--min-psnr", "NaN", expected=2)
    run("optimize", files[0], "--min-ssim", "1", "--out-dir", out / "strict")
args.report.parent.mkdir(parents=True, exist_ok=True)
args.report.write_text(json.dumps(records, indent=2), encoding="utf-8")
print(f"Passed {len(records)} CLI checks; report: {args.report}")
