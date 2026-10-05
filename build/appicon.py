# Draws build/appicon.png, the launcher icon: a bone-coloured sword over a
# thin gold ring, on a deep red tile. Drawn at 4x and scaled down for smooth
# edges; each part is its own layer so translucent parts blend. Needs Pillow.
#
#   python build/appicon.py build/appicon.png
#   wails3 task common:generate:icons
#
# build/appicon.icon/Assets/sword.svg is the same sword for macOS.
import sys
from PIL import Image, ImageDraw, ImageFilter

out = sys.argv[1]
N = 4096
k = N / 1024  # design units are for 1024px


def s(*v):
    return [x * k for x in v]


def layer():
    im = Image.new("RGBA", (N, N), (0, 0, 0, 0))
    return im, ImageDraw.Draw(im)


tile_box = s(64, 64, 960, 960)
radius = 200 * k

# Tile: radial gradient from #8c1111 to #1c0202.
grad, g = layer()
cx, cy = N * 0.5, N * 0.45
for i in range(500, 0, -1):
    t = (i / 500) ** 0.9
    r = N * 0.72 * (i / 500)
    c = (int(0x8C * (1 - t) + 0x1C * t), int(0x11 * (1 - t) + 0x02 * t), int(0x11 * (1 - t) + 0x02 * t), 255)
    g.ellipse([cx - r, cy - r, cx + r, cy + r], fill=c)
mask = Image.new("L", (N, N), 0)
ImageDraw.Draw(mask).rounded_rectangle(tile_box, radius=radius, fill=255)
img = Image.new("RGBA", (N, N), (0, 0, 0, 0))
img.paste(grad, (0, 0), mask)

# Faint inner edge.
edge, e = layer()
e.rounded_rectangle(s(80, 80, 944, 944), radius=186 * k, outline=(255, 200, 170, 36), width=int(5 * k))
img = Image.alpha_composite(img, edge)

# Ring.
ring, r = layer()
r.ellipse(s(236, 250, 788, 802), outline=(214, 178, 110, 120), width=int(12 * k))
img = Image.alpha_composite(img, ring)

bone = (238, 227, 203, 255)
bone_dark = (196, 180, 150, 255)
leather = (96, 58, 38, 255)
gold = (205, 168, 98, 255)


def sword(d, shadow=False):
    c = (0, 0, 0, 150) if shadow else None
    # Blade, in two halves for a bevel.
    d.polygon(s(512, 884, 468, 768, 468, 352, 512, 352), fill=c or bone)
    d.polygon(s(512, 884, 556, 768, 556, 352, 512, 352), fill=c or bone_dark)
    # Crossguard with flared ends.
    d.polygon(s(292, 318, 732, 318, 756, 346, 732, 374, 292, 374, 268, 346), fill=c or gold)
    # Grip and pommel.
    d.rounded_rectangle(s(490, 180, 534, 320), radius=8 * k, fill=c or leather)
    d.ellipse(s(470, 120, 554, 204), fill=c or gold)


shadow, sd = layer()
sword(sd, shadow=True)
shadow = shadow.transform(shadow.size, Image.AFFINE, (1, 0, -14 * k, 0, 1, -18 * k)).filter(ImageFilter.GaussianBlur(18 * k))
shadow.putalpha(Image.composite(shadow.getchannel("A"), Image.new("L", (N, N), 0), mask))
img = Image.alpha_composite(img, shadow)

blade, bd = layer()
sword(bd)
img = Image.alpha_composite(img, blade)

img.resize((1024, 1024), Image.LANCZOS).save(out)
