"""Private, resumable PNG/JPEG corpus validation; reports contain source paths."""
import argparse
import ctypes
from ctypes import wintypes
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from compare_png import accept

POLICY = {"minGain": 10, "minPSNR": 30, "minSSIM": .95}


def sha(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def execute(argv, timeout=300):
    started = time.perf_counter()
    proc = subprocess.Popen(list(map(str, argv)), stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    timed_out = False
    try:
        out, err = proc.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        proc.kill()
        out, err = proc.communicate()
    peak = None
    if os.name == "nt":
        class Counters(ctypes.Structure):
            _fields_ = [("cb", wintypes.DWORD), ("PageFaultCount", wintypes.DWORD)] + [
                (name, ctypes.c_size_t) for name in ("PeakWorkingSetSize", "WorkingSetSize",
                "QuotaPeakPagedPoolUsage", "QuotaPagedPoolUsage", "QuotaPeakNonPagedPoolUsage",
                "QuotaNonPagedPoolUsage", "PagefileUsage", "PeakPagefileUsage")]
        counter = Counters()
        counter.cb = ctypes.sizeof(counter)
        api = ctypes.WinDLL("psapi", use_last_error=True).GetProcessMemoryInfo
        api.argtypes = [wintypes.HANDLE, ctypes.POINTER(Counters), wintypes.DWORD]
        api.restype = wintypes.BOOL
        if api(wintypes.HANDLE(int(proc._handle)), ctypes.byref(counter), counter.cb):
            peak = counter.PeakWorkingSetSize
    result = {"exit": proc.returncode, "seconds": time.perf_counter() - started,
              "peakWorkingSetBytes": peak, "timedOut": timed_out}
    try:
        result["report"] = json.loads(out.decode("utf-8-sig"))
    except (ValueError, UnicodeError):
        result["stdout"] = out.decode("utf-8", errors="replace")
    if err:
        result["stderr"] = err.decode("utf-8", errors="replace")
    return result


def checkpoint(path, data):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(data, indent=2, allow_nan=False), encoding="utf-8")
    tmp.replace(path)


def sample(items, count):
    groups = {}
    for item in items:
        m = item["metrics"]
        pixels = m["width"] * m["height"]
        key = (m["format"], m.get("hasAlpha", False),
               bool(set(m.get("chunks", [])) & {"ICC_PROFILE", "iCCP"}),
               0 if pixels < 1000000 else 1 if pixels < 8000000 else 2)
        groups.setdefault(key, []).append(item)
    for group in groups.values():
        group.sort(key=lambda i: i["bytesBefore"])
    selected = []
    while len(selected) < min(count, len(items)):
        for key in sorted(groups):
            if groups[key] and len(selected) < count:
                selected.append(groups[key].pop(len(groups[key]) // 2))
    return selected


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("corpus", type=Path)
    p.add_argument("output", type=Path)
    p.add_argument("--bin", type=Path, default=Path("bin"))
    p.add_argument("--sample", type=int, default=0)
    a = p.parse_args()
    corpus, output = a.corpus.resolve(), a.output.resolve()
    if output == corpus or corpus in output.parents:
        p.error("output must be outside source tree")
    if a.sample < 0:
        p.error("--sample must be nonnegative")
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()) and not (output / "manifest.json").exists():
        p.error("use an empty output directory for a new run")
    suffix = ".exe" if os.name == "nt" else ""
    binaries = {n: (a.bin / (n + suffix)).resolve() for n in
                ["pixelita", "img-scan", "img-quant", "img-jpeg"]}
    reference = shutil.which("pngquant")
    if not reference:
        p.error("pngquant not found")
    signature = {k: sha(v) for k, v in binaries.items()}
    signature["pngquant"] = sha(reference)
    manifest_path = output / "manifest.json"
    if manifest_path.exists():
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        if (manifest["binaries"] != signature or manifest["corpus"] != str(corpus)
                or manifest["policy"] != POLICY or manifest["jpegMinGain"] != 1):
            p.error("resume requires identical binaries and corpus")
    else:
        inventory = execute([binaries["img-scan"], "-quick", "-json", corpus])
        if inventory["exit"] != 0:
            raise RuntimeError(inventory)
        items = inventory["report"]["items"]
        for index, item in enumerate(items):
            item["id"] = f"image-{index:03d}"
            item["sha256"] = sha(item["path"])
        manifest = {"corpus": str(corpus), "binaries": signature,
                    "policy": POLICY, "jpegMinGain": 1, "items": items}
        checkpoint(manifest_path, manifest)
    selected = sample(manifest["items"], a.sample) if a.sample else manifest["items"]
    checkpoint(output / "selection.json", [i["id"] for i in selected])
    for item in selected:
        identity, source = item["id"], Path(item["path"])
        result_path = output / (identity + ".json")
        if result_path.exists():
            continue
        if sha(source) != item["sha256"]:
            raise RuntimeError(f"source changed: {identity}")
        directory = output / identity
        directory.mkdir(exist_ok=True)
        result = {"id": identity, "format": item["metrics"]["format"],
                  "bytesBefore": item["bytesBefore"]}
        png = result["format"] == "png"
        cmd = [binaries["img-quant" if png else "img-jpeg"], "-json",
               "-out-dir", directory, "-overwrite", "-min-gain", "10" if png else "1"]
        if png:
            cmd += ["-min-psnr", "30", "-min-ssim", ".95"]
        result["pixelita"] = execute(cmd + [source])
        reported = result["pixelita"].get("report", {}).get("items", [])
        if reported and reported[0].get("written"):
            candidate = reported[0]["output"]
            result["verification"] = execute([binaries["pixelita"], "compare", "--json",
                "--strict-size", "--min-psnr", "30" if png else "200",
                "--min-ssim", ".95" if png else "1", source, candidate])
            result["outputInventory"] = execute([binaries["img-scan"], "-quick", "-json", candidate])
        if png:
            reference_output = directory / "reference.png"
            # An interrupted run may leave this private reference output behind.
            if reference_output.exists():
                reference_output.unlink()
            result["pngquant"] = execute([reference, "--speed", "4", "--floyd=1",
                "--output", reference_output, "256", "--", source])
            if result["pngquant"]["exit"] == 0:
                measured = execute([binaries["pixelita"], "compare", "--json",
                                    "--strict-size", source, reference_output])
                result["referenceVerification"] = measured
                if measured["exit"] == 0:
                    metrics = measured["report"]["items"][0]["metrics"]
                    result["referenceBytes"] = reference_output.stat().st_size
                    result["referenceAccepted"] = accept(result["referenceBytes"],
                                                        item["bytesBefore"], metrics, POLICY)
        result["sourceUnchanged"] = sha(source) == item["sha256"]
        if not result["sourceUnchanged"]:
            raise RuntimeError(f"source changed: {identity}")
        checkpoint(result_path, result)
        status = reported[0].get("status") if reported else "process failure"
        print(f"{identity}: {result['format']} {status}, "
              f"{result['pixelita']['seconds']:.1f}s, "
              f"peak={result['pixelita']['peakWorkingSetBytes']}", flush=True)
    unchanged = all(sha(i["path"]) == i["sha256"] for i in manifest["items"])
    checkpoint(output / "integrity.json", {"allSourcesUnchanged": unchanged,
               "files": len(manifest["items"]), "checkedAtUnix": time.time()})
    if not unchanged:
        raise RuntimeError("source integrity check failed")


if __name__ == "__main__":
    main()
