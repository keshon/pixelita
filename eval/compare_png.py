"""Compare PNG candidates at shared gain/RGB/alpha/SSIM floors.

Pixelita preview includes verification; pngquant timing excludes verification.
These are workflow timings, not comparable codec-only speed measurements.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import platform
import shutil
import statistics
import subprocess
import tempfile
import time


def run(argv):
    start = time.perf_counter()
    p = subprocess.run(list(map(str, argv)), capture_output=True, text=True, timeout=300)
    if p.returncode:
        raise RuntimeError(f"{argv[0]} exited {p.returncode}: {p.stderr or p.stdout}")
    return p.stdout, time.perf_counter() - start


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def accept(size, before, metrics, policy):
    return (100 * (before - size) / before >= policy["minGain"]
            and metrics.get("psnr", math.inf) >= policy["minPSNR"]
            and metrics.get("psnrAlpha", math.inf) >= policy["minPSNR"]
            and metrics["ssim"] >= policy["minSSIM"])


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--bin", type=Path, default=Path("bin"))
    p.add_argument("--pngquant", default="pngquant")
    p.add_argument("--report", type=Path, required=True)
    p.add_argument("--repeat", type=int, default=3)
    p.add_argument("--min-gain", type=float, default=10)
    p.add_argument("--min-psnr", type=float, default=30)
    p.add_argument("--min-ssim", type=float, default=.95)
    p.add_argument("inputs", type=Path, nargs="+")
    a = p.parse_args()
    if a.repeat < 1:
        p.error("--repeat must be positive")
    for value, maximum in [(a.min_gain, 100), (a.min_psnr, math.inf), (a.min_ssim, 1)]:
        if not math.isfinite(value) or not 0 <= value <= maximum:
            p.error("invalid policy threshold")
    inputs = sorted({x.resolve() for arg in a.inputs for x in
                     (arg.rglob("*.png") if arg.is_dir() else [arg])})
    if not inputs or any(not x.is_file() or x.suffix.lower() != ".png" for x in inputs):
        p.error("provide existing PNG files or directories containing PNG files")
    suffix = ".exe" if platform.system() == "Windows" else ""
    pixelita = (a.bin / ("pixelita" + suffix)).resolve()
    quant = (a.bin / ("img-quant" + suffix)).resolve()
    reference = shutil.which(a.pngquant)
    if not reference:
        p.error("pngquant was not found")
    policy = dict(minGain=a.min_gain, minPSNR=a.min_psnr, minSSIM=a.min_ssim)
    report = {
        "schema": 1, "policy": policy, "repetitions": a.repeat,
        "platform": platform.platform(),
        "pixelitaVersion": run([quant, "-version"])[0].strip(),
        "pixelitaBinarySHA256": digest(pixelita),
        "quantBinarySHA256": digest(quant),
        "pngquantVersion": run([reference, "--version"])[0].strip(),
        "settings": {"pixelita": "256 colours, dither 1, effort 6",
                     "pngquant": "256 colours, floyd=1, speed=4; no quality search"},
        "limitations": [
            "Fixed settings, common acceptance floors; not equal achieved quality.",
            "Pixelita preview includes decode/encode/verification; pngquant encode timing does not.",
            "Missing PSNR means positive infinity only in successfully measured reports.",
            "No metadata equivalence, peak-memory or perceptual quality claim.",
            "Rejected candidates count as retained originals in effective byte totals."
        ], "items": []
    }
    with tempfile.TemporaryDirectory(prefix="pixelita-reference-") as tmp:
        for index, source in enumerate(inputs):
            before_hash = digest(source)
            before = source.stat().st_size
            row = {"name": source.name, "sha256": before_hash, "bytesBefore": before, "runs": []}
            for repetition in range(a.repeat):
                output = Path(tmp) / f"{index}-{repetition}.png"
                # Alternate order to reduce a systematic warm-cache advantage.
                results = {}
                for tool in (["pixelita", "pngquant"] if repetition % 2 == 0 else ["pngquant", "pixelita"]):
                    if tool == "pixelita":
                        raw, seconds = run([quant, "-json", "-dry-run", "-colors", "256",
                            "-dither", "1", "-effort", "6", "-min-gain", "0",
                            "-min-psnr", "0", "-min-ssim", "0", source])
                        item = json.loads(raw)["items"][0]
                        size, metrics = item["bytesAfter"], item["metrics"]
                        verification_seconds = None
                    else:
                        _, seconds = run([reference, "--speed", "4", "--floyd=1",
                                          "--output", output, "256", "--", source])
                        raw, verification_seconds = run([pixelita, "compare", "--json",
                                                          "--strict-size", source, output])
                        item = json.loads(raw)["items"][0]
                        size, metrics = output.stat().st_size, item["metrics"]
                    if item.get("verification", {}).get("status") != "measured":
                        raise RuntimeError(f"missing verification for {source}")
                    accepted = accept(size, before, metrics, policy)
                    results[tool] = dict(bytes=size, metrics=metrics, accepted=accepted,
                        effectiveBytes=size if accepted else before, seconds=seconds,
                        externalVerificationSeconds=verification_seconds)
                row["runs"].append(results)
            if digest(source) != before_hash:
                raise RuntimeError(f"source changed: {source}")
            report["items"].append(row)
            print(f"{source.name}: " + ", ".join(
                f"{tool} {row['runs'][0][tool]['bytes']} bytes, accepted={row['runs'][0][tool]['accepted']}"
                for tool in ["pixelita", "pngquant"]))
    report["totals"] = {}
    for tool in ["pixelita", "pngquant"]:
        report["totals"][tool] = {
            "effectiveBytes": sum(statistics.median(r[tool]["effectiveBytes"] for r in x["runs"]) for x in report["items"]),
            "acceptedRuns": sum(r[tool]["accepted"] for x in report["items"] for r in x["runs"]),
            "runs": len(inputs) * a.repeat,
        }
    report["totals"]["inputBytes"] = sum(x["bytesBefore"] for x in report["items"])
    a.report.parent.mkdir(parents=True, exist_ok=True)
    a.report.write_text(json.dumps(report, indent=2, allow_nan=False) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
