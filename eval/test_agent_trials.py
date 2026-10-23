"""Exercise artifact graders without claiming these scripted runs are agent trials."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

from agent_trials import grade, sha


class ArtifactGraderTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix="pixelita-grader-")
        cls.base = Path(cls.temp.name)
        cls.repo = Path(__file__).resolve().parents[1]
        cls.binary = cls.repo / "bin/pixelita.exe"
        cls.corpus = cls.base / "fixtures"
        subprocess.run([sys.executable,str(cls.repo/"eval/generate_corpus.py"),str(cls.corpus)],check=True)

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def workspace(self, name):
        root=self.base/name
        (root/"inputs").mkdir(parents=True)
        (root/"out").mkdir()
        return root

    def invoke(self,*args,expected=0):
        p=subprocess.run([str(self.binary),*map(str,args),"--json"],capture_output=True,text=True)
        self.assertEqual(p.returncode,expected,p.stderr or p.stdout)
        return json.loads(p.stdout)

    def test_optimization_and_source_integrity(self):
        root=self.workspace("optimize")
        for name in ["flat-rgba.png","photo-rgb.png","baseline.jpg"]:
            shutil.copy2(self.corpus/name,root/"inputs"/name)
        hashes={p:sha(p) for p in (root/"inputs").iterdir()}
        self.assertFalse(grade(root,"optimize",hashes,self.binary)["allOutputsMeetPolicy"])
        self.invoke("optimize",root/"inputs","--out-dir",root/"out","--min-gain","1","--min-psnr","30","--min-ssim",".95")
        self.invoke("optimize",root/"inputs","--out-dir",root/"out","--min-gain","1","--min-psnr","30","--min-ssim",".95","--apply")
        self.assertTrue(all(grade(root,"optimize",hashes,self.binary).values()))
        next(iter(hashes)).write_bytes(b"changed")
        self.assertFalse(grade(root,"optimize",hashes,self.binary)["sourcesUnchanged"])

    def test_collision_recovery(self):
        root=self.workspace("collision")
        for side in ["left","right"]:
            shutil.copytree(self.corpus/side,root/"inputs"/side)
        sentinel=root/"out/same-min.png"
        sentinel.write_bytes(b"keep this")
        hashes={p:sha(p) for p in (root/"inputs").rglob("*.png")}
        hashes[sentinel]=sha(sentinel)
        before={p:sha(p) for p in (root/"out").iterdir()}
        self.invoke("optimize",root/"inputs","--out-dir",root/"out",expected=2)
        self.assertEqual(before,{p:sha(p) for p in (root/"out").iterdir()})
        for side in ["left","right"]:
            self.invoke("optimize",root/"inputs"/side,"--out-dir",root/"out"/side,"--min-psnr","30","--min-ssim",".95","--apply")
        self.assertTrue(all(grade(root,"collision",hashes,self.binary).values()))
        next((root/"out/right").glob("*.png")).unlink()
        self.assertFalse(grade(root,"collision",hashes,self.binary)["allOutputsMeetPolicy"])

    def test_damage_region_and_preview(self):
        from PIL import Image, ImageDraw
        root=self.workspace("damage")
        shutil.copy2(self.corpus/"photo-rgb.png",root/"inputs/before.png")
        im=Image.open(root/"inputs/before.png").convert("RGB")
        ImageDraw.Draw(im).rectangle((384,256,511,383),fill=(0,0,0))
        im.save(root/"inputs/after.png")
        hashes={p:sha(p) for p in (root/"inputs").iterdir()}
        r=self.invoke("compare",root/"inputs/before.png",root/"inputs/after.png","--worst","1")
        region=r["items"][0]["metrics"]["crop"]
        self.invoke("view",root/"inputs/before.png",root/"inputs/after.png","--crop",region,"--zoom","2","--out",root/"out/preview.png")
        result={"region":region,"preview":"out/preview.png"}
        (root/"result.json").write_text(json.dumps(result))
        self.assertTrue(all(grade(root,"damage",hashes,self.binary).values()))
        result["region"]="0,0,16,16"
        (root/"result.json").write_text(json.dumps(result))
        self.assertFalse(grade(root,"damage",hashes,self.binary)["damageLocated"])


if __name__=="__main__":unittest.main()
