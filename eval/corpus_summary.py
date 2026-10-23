"""Produce a path-free corpus summary and independently verify retained metadata."""
import argparse
from collections import Counter
import json
from pathlib import Path
import statistics

from corpus_check import sha, checkpoint


def metadata(path, fmt):
    raw = Path(path).read_bytes()
    result = []
    if fmt == "png":
        if raw[:8] != b"\x89PNG\r\n\x1a\n":
            raise ValueError("invalid PNG")
        off = 8
        while off + 12 <= len(raw):
            size = int.from_bytes(raw[off:off+4], "big")
            end = off + size + 12
            if end > len(raw):
                raise ValueError("truncated PNG")
            name = raw[off+4:off+8]
            if name not in (b"IHDR", b"IDAT", b"IEND", b"PLTE", b"tRNS"):
                result.append(raw[off:end])
            off = end
            if name == b"IEND":
                break
    else:
        if raw[:2] != b"\xff\xd8":
            raise ValueError("invalid JPEG")
        off = 2
        while off < len(raw):
            if raw[off] != 255:
                raise ValueError("invalid JPEG marker")
            while raw[off] == 255:
                off += 1
            marker = raw[off]
            off += 1
            if marker in (0xDA, 0xD9):
                break
            if marker == 1 or 0xD0 <= marker <= 0xD7:
                continue
            size = int.from_bytes(raw[off:off+2], "big")
            if size < 2 or off + size > len(raw):
                raise ValueError("truncated JPEG")
            if 0xE0 <= marker <= 0xEF or marker == 0xFE:
                result.append(bytes([marker]) + raw[off:off+size])
            off += size
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("directory", type=Path)
    p.add_argument("--out", type=Path, required=True)
    a = p.parse_args()
    manifest = json.loads((a.directory / "manifest.json").read_text(encoding="utf-8"))
    summary = {
        "schema": 1, "policy": manifest["policy"], "jpegMinGain": manifest["jpegMinGain"],
        "inputCount": len(manifest["items"]), "completed": 0, "formats": {},
        "allSourcesUnchanged": True, "writtenOutputsVerified": 0,
        "metadataExactMatches": 0, "metadataMismatches": 0, "verificationFailures": 0,
        "limitations": [
            "One sequential run per image, including a stratified 24-image smoke sample.",
            "Windows peak working set per child process, not simultaneous batch memory.",
            "Pixelita includes verification and publication; pngquant encoding excludes comparison.",
            "Reference acceptance tests pixels and bytes, not metadata equivalence.",
            "Spot checks are not exhaustive perceptual validation.",
            "Corpus contains related screenshots; 144 unique files are not 144 independent content classes."
        ],
    }
    for fmt in ("png", "jpeg"):
        originals = [i for i in manifest["items"] if i["metrics"]["format"] == fmt]
        stats = {"inputs": len(originals), "inputBytes": sum(i["bytesBefore"] for i in originals),
                 "completed": 0, "completedInputBytes": 0, "written": 0, "effectiveBytes": 0, "status": Counter(),
                 "skipCodes": Counter(), "failureReasons": Counter(),
                 "seconds": [], "peaks": [], "referenceAccepted": 0,
                 "referenceCompleted": 0, "referenceFailures": 0, "referenceEffectiveBytes": 0,
                 "referenceEncodeSeconds": [], "referenceVerifySeconds": [],
                 "referencePeaks": [], "referenceVerifyPeaks": []}
        for original in originals:
            if sha(original["path"]) != original["sha256"]:
                summary["allSourcesUnchanged"] = False
            record = a.directory / (original["id"] + ".json")
            if not record.exists():
                continue
            r = json.loads(record.read_text(encoding="utf-8"))
            stats["completed"] += 1
            stats["completedInputBytes"] += original["bytesBefore"]
            summary["completed"] += 1
            run = r["pixelita"]
            stats["seconds"].append(run["seconds"])
            if run["peakWorkingSetBytes"] is not None:
                stats["peaks"].append(run["peakWorkingSetBytes"])
            items = run.get("report", {}).get("items", [])
            item = items[0] if items else {}
            status = item.get("status", "process_failure")
            stats["status"][status] += 1
            written = item.get("written", False)
            stats["effectiveBytes"] += item["bytesAfter"] if written else original["bytesBefore"]
            if written:
                stats["written"] += 1
                verified = r.get("verification", {}).get("exit") == 0
                summary["writtenOutputsVerified" if verified else "verificationFailures"] += 1
                equal = metadata(original["path"], fmt) == metadata(item["output"], fmt)
                summary["metadataExactMatches" if equal else "metadataMismatches"] += 1
            elif status == "skipped":
                stats["skipCodes"][item.get("code", "unknown")] += 1
            else:
                # Only controlled reason labels; never include path-bearing error text.
                reason = "metadata preservation unsupported" if item.get("reason") == "metadata preservation unsupported" else "operation or process failure"
                stats["failureReasons"][reason] += 1
            if fmt == "png":
                ref = r.get("pngquant", {})
                if "referenceAccepted" in r:
                    stats["referenceCompleted"] += 1
                    accepted = r["referenceAccepted"]
                    stats["referenceAccepted"] += int(accepted)
                    stats["referenceEffectiveBytes"] += r["referenceBytes"] if accepted else original["bytesBefore"]
                else:
                    stats["referenceFailures"] += 1
                    stats["referenceEffectiveBytes"] += original["bytesBefore"]
                if "seconds" in ref:
                    stats["referenceEncodeSeconds"].append(ref["seconds"])
                if ref.get("peakWorkingSetBytes") is not None:
                    stats["referencePeaks"].append(ref["peakWorkingSetBytes"])
                if "referenceVerification" in r:
                    stats["referenceVerifySeconds"].append(r["referenceVerification"]["seconds"])
                    peak = r["referenceVerification"].get("peakWorkingSetBytes")
                    if peak is not None:
                        stats["referenceVerifyPeaks"].append(peak)
        for key in ("seconds", "peaks", "referenceEncodeSeconds", "referenceVerifySeconds", "referencePeaks", "referenceVerifyPeaks"):
            values = stats.pop(key)
            stats[key] = {"median": statistics.median(values), "max": max(values)} if values else None
            if values and "peak" not in key.lower():
                stats[key]["sum"] = sum(values)
        summary["formats"][fmt] = stats
    checkpoint(a.out, summary)
    print(json.dumps(summary, indent=2))
    if (not summary["allSourcesUnchanged"] or summary["metadataMismatches"]
            or summary["verificationFailures"]):
        raise RuntimeError("corpus integrity or output verification failed")


if __name__ == "__main__":
    main()
