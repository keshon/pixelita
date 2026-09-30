from pathlib import Path
import argparse
import random

from PIL import Image, ImageDraw, ImageFilter


def patterned(size: tuple[int, int], seed: int) -> Image.Image:
    rng = random.Random(seed)
    width, height = size
    image = Image.new("RGB", size)
    pixels = image.load()
    for y in range(height):
        for x in range(width):
            grain = rng.randrange(-14, 15)
            pixels[x, y] = (
                max(0, min(255, 24 + 190 * x // width + grain)),
                max(0, min(255, 34 + 160 * y // height + grain)),
                max(0, min(255, 45 + 90 * (x + y) // (width + height) + grain)),
            )
    image = image.filter(ImageFilter.GaussianBlur(0.45))
    draw = ImageDraw.Draw(image)
    for _ in range(80):
        x = rng.randrange(width)
        y = rng.randrange(height)
        radius = rng.randrange(8, 70)
        colour = tuple(rng.randrange(30, 230) for _ in range(3))
        draw.ellipse((x - radius, y - radius, x + radius, y + radius), fill=colour)
    return image


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    root = args.output
    for child in (root, root / "left", root / "right"):
        child.mkdir(parents=True, exist_ok=True)

    rgba = Image.new("RGBA", (640, 480), (0, 0, 0, 0))
    draw = ImageDraw.Draw(rgba)
    for y in range(480):
        draw.rectangle((0, y, 639, y), fill=(24, 84 + y // 4, 170, 80 + y // 3))
    draw.rounded_rectangle((70, 55, 570, 425), radius=56, fill=(250, 190, 45, 220))
    draw.text((190, 215), "PIXELITA", fill=(20, 30, 45, 255))
    rgba.save(root / "flat-rgba.png", optimize=False)

    photo = patterned((1280, 853), 20260930)
    photo.save(root / "photo-rgb.png", optimize=False)
    photo.save(root / "baseline.jpg", quality=92, progressive=False, subsampling=0)
    photo.save(root / "progressive.jpg", quality=92, progressive=True, subsampling=0)
    patterned((360, 240), 1).save(root / "left" / "same.png")
    patterned((360, 240), 2).save(root / "right" / "same.png")
    (root / "unsupported.txt").write_text("not an image\n", encoding="utf-8")
    (root / "corrupt.png").write_bytes(b"not a png")


if __name__ == "__main__":
    main()
