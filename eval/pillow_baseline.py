from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

from PIL import Image


def psnr(before: Image.Image, after: Image.Image) -> float | None:
    a = before.convert("RGBA")
    b = after.convert("RGBA")
    if a.size != b.size:
        return None
    total = sum((x - y) ** 2 for x, y in zip(a.tobytes(), b.tobytes()))
    if total == 0:
        return math.inf
    mse = total / (a.width * a.height * 4)
    return 10 * math.log10((255 * 255) / mse)


def optimize(source: Path, output: Path) -> dict[str, object]:
    with Image.open(source) as image:
        image.load()
        output.parent.mkdir(parents=True, exist_ok=True)
        if source.suffix.lower() == ".png":
            method = Image.Quantize.FASTOCTREE if image.mode == "RGBA" else Image.Quantize.MEDIANCUT
            candidate = image.quantize(colors=256, method=method, dither=Image.Dither.FLOYDSTEINBERG)
            candidate.save(output, format="PNG", optimize=True)
        else:
            image.save(
                output,
                format="JPEG",
                quality="keep",
                subsampling="keep",
                qtables="keep",
                optimize=True,
                progressive=False,
            )
    with Image.open(source) as before_image, Image.open(output) as after_image:
        fidelity = psnr(before_image, after_image)
    before_bytes = source.stat().st_size
    after_bytes = output.stat().st_size
    return {
        "source": str(source),
        "output": str(output),
        "beforeBytes": before_bytes,
        "afterBytes": after_bytes,
        "gainPercent": (1 - after_bytes / before_bytes) * 100,
        "psnr": "inf" if fidelity is not None and math.isinf(fidelity) else fidelity,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    parser.add_argument("inputs", nargs="+", type=Path)
    args = parser.parse_args()
    results = [optimize(source, args.output / source.name) for source in args.inputs]
    print(json.dumps({"tool": "pillow-baseline", "items": results}, indent=2))


if __name__ == "__main__":
    main()
