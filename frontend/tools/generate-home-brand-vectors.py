"""Regenerate inline brand outlines; dev-only: pip install fonttools.
No font parser or font download is needed by the browser.
"""
from pathlib import Path
import json
import math
from fontTools.ttLib import TTFont
from fontTools.pens.basePen import BasePen

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / 'src/assets/home'
SAMPLES = 96
# Latin-1 plus the existing wordmark repertoire's accents and punctuation.
LATIN_EXTRA = set('ıŒœʻʼˆ˚˜\u0300\u0301\u0303\u0304\u0308\u0309\u0323–—‘’‚“”„•…′″‹›⁄€™−')

class OutlinePen(BasePen):
    def __init__(self, glyphs=None):
        super().__init__(glyphs)
        self.rings = []
        self.points = []
    def _moveTo(self, p):
        self.points = [p]
    def _lineTo(self, p):
        self.points.append(p)
    def _curveToOne(self, a, b, c):
        p = self.points[-1]
        for i in range(1, 25):
            t = i / 24
            self.points.append(tuple((1-t)**3*p[j]+3*(1-t)**2*t*a[j]+3*(1-t)*t*t*b[j]+t**3*c[j] for j in (0,1)))
    def _qCurveToOne(self, a, b):
        p = self.points[-1]
        for i in range(1, 17):
            t = i / 16
            self.points.append(tuple((1-t)**2*p[j]+2*(1-t)*t*a[j]+t*t*b[j] for j in (0,1)))
    def _closePath(self):
        if len(self.points) > 2:
            self.rings.append(self.points)
        self.points = []
    def _endPath(self):
        self._closePath()

def sample(points):
    points = points + [points[0]]
    lengths = [math.dist(a,b) for a,b in zip(points,points[1:])]
    total = sum(lengths)
    result = []
    j = 0
    passed = 0
    for i in range(SAMPLES):
        d = total*i/SAMPLES
        while j < len(lengths)-1 and passed+lengths[j] < d:
            passed += lengths[j]
            j += 1
        t = (d-passed)/lengths[j] if lengths[j] else 0
        result += [round(points[j][k]+t*(points[j+1][k]-points[j][k]),1) for k in (0,1)]
    return result

font = TTFont(ASSETS / 'lora-regular.ttf')
glyphs = font.getGlyphSet()
characters = {}
for code, name in font.getBestCmap().items():
    if code < 32 or (code > 255 and chr(code) not in LATIN_EXTRA): continue
    pen = OutlinePen(glyphs)
    glyphs[name].draw(pen)
    characters[chr(code)] = {'advance': glyphs[name].width, 'rings': [sample([(x,-y) for x,y in r]) for r in pen.rings]}
# Half-unit delta encoding and a shared contour pool avoid repeating accented glyphs.
pool = []
lookup = {}
def pack(ring):
    values = [round(v*2) for v in ring]
    encoded = values[:2] + [values[i]-values[i-2] for i in range(2,len(values))]
    key = tuple(encoded)
    if key not in lookup:
        lookup[key] = len(pool)
        pool.append(encoded)
    return lookup[key]
for glyph in characters.values():
    glyph['rings'] = [pack(r) for r in glyph['rings']]
output = {'glyphs':characters,'rings':pool}
(ASSETS / 'brand-vectors.json').write_text(json.dumps(output,ensure_ascii=False,separators=(',',':'))+'\n')
print('Wrote', len(characters), 'glyphs;', (ASSETS / 'brand-vectors.json').stat().st_size, 'bytes')
