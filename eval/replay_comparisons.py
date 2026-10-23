"""Replay saved corpus comparisons after a metric change, requiring exact parity."""
import argparse
import json
import os
from pathlib import Path
from corpus_check import execute, checkpoint, sha

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("directory", type=Path)
p.add_argument("--binary", type=Path, default=Path("bin/pixelita.exe" if os.name == "nt" else "bin/pixelita"))
a = p.parse_args()
manifest = json.loads((a.directory/"manifest.json").read_text(encoding="utf-8"))
summary = {"comparisons": 0, "exactMetricMatches": 0, "statusMatches": 0,
           "sourceIntegrity": True, "binarySHA256": sha(a.binary),
           "maxPeakBefore": 0, "maxPeakAfter": 0, "pairs": []}
for original in manifest["items"]:
    if sha(original["path"]) != original["sha256"]:
        raise RuntimeError("source changed")
    record = json.loads((a.directory/(original["id"]+".json")).read_text(encoding="utf-8"))
    for key in ("verification", "referenceVerification"):
        if key not in record:
            continue
        old = record[key]
        if old["exit"] != 0:
            raise RuntimeError("cannot replay an unsuccessful baseline")
        item = old["report"]["items"][0]
        argv = [a.binary.resolve(), "compare", "--json", "--strict-size"]
        if key == "verification":
            argv += ["--min-psnr", "200" if record["format"]=="jpeg" else "30",
                     "--min-ssim", "1" if record["format"]=="jpeg" else ".95"]
        new = execute(argv + [item["path"], item["output"]])
        metrics = new.get("report",{}).get("items",[{}])[0].get("metrics")
        same = metrics == item["metrics"]
        status_same = new["exit"] == old["exit"]
        summary["comparisons"] += 1
        summary["exactMetricMatches"] += int(same)
        summary["statusMatches"] += int(status_same)
        before, after = old.get("peakWorkingSetBytes"), new.get("peakWorkingSetBytes")
        summary["maxPeakBefore"] = max(summary["maxPeakBefore"],before or 0)
        summary["maxPeakAfter"] = max(summary["maxPeakAfter"],after or 0)
        summary["pairs"].append({"id":original["id"],"kind":key,"same":same,
            "beforePeak":before,"afterPeak":after,"beforeSeconds":old["seconds"],"afterSeconds":new["seconds"]})
        checkpoint(a.directory/"replay.json",summary)
        if not same or not status_same:
            raise RuntimeError(f"comparison changed: {original['id']} {key}")
    print(original["id"],"checked",flush=True)
print(json.dumps({k:v for k,v in summary.items() if k!="pairs"},indent=2))
