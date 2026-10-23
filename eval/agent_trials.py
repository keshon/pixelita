"""Run isolated, independently graded Pixelita agent trials using codex exec."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

from PIL import Image, ImageDraw

TASKS = {
"optimize": """Optimize all images in inputs into out, keeping formats and dimensions. Require at least 1% saving, 30 dB PSNR and 0.95 SSIM; JPEGs must remain pixel-identical. Preview first, apply accepted candidates, then verify written outputs. Leave inputs untouched. Finish with a concise factual summary.""",
"collision": """Two PNG files under inputs have the same basename. First attempt an optimization preview targeting out. Recover from any destination conflict without overwriting or deleting anything. Optimize both into distinct locations under out at 10% minimum saving, 30 dB PSNR and 0.95 SSIM. Keep the pre-existing out/same-min.png unchanged. Verify both written outputs and summarize.""",
"damage": """Compare inputs/before.png and inputs/after.png. Locate the worst damaged region, create a magnified before/after preview inside out, and write result.json containing region (a string x,y,w,h) and preview (a workspace-relative path). Leave both inputs untouched. Use Pixelita's measurements, not a hand-written image metric."""
}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def cli(binary, args):
    p = subprocess.run([str(binary), *map(str,args)], capture_output=True, text=True, timeout=120)
    return p.returncode, json.loads(p.stdout)


def grade(root, task, hashes, pixelita):
    checks={"sourcesUnchanged": all(p.exists() and sha(p)==h for p,h in hashes.items())}
    if task == "damage":
        try:
            r=json.loads((root/"result.json").read_text(encoding="utf-8-sig"))
            x,y,w,h=map(int,r["region"].split(","))
            checks["damageLocated"]=w>0 and h>0 and x<512 and x+w>384 and y<384 and y+h>256
            preview=(root/r["preview"]).resolve()
            checks["previewCreated"]=root.resolve() in preview.parents and preview.is_file()
            if checks["previewCreated"]:
                with Image.open(preview) as im:
                    checks["previewCreated"]=im.format=="PNG" and im.width>=w and im.height>=h
        except (OSError,ValueError,KeyError,TypeError):
            checks["damageLocated"]=False
            checks["previewCreated"]=False
    else:
        candidates=[p for p in (root/"out").rglob("*") if p.is_file() and p not in hashes and p.suffix.lower() in (".png",".jpg",".jpeg")]
        inputs=[p for p in hashes if root/"inputs" in p.parents]
        matches={}
        for source in inputs:
            matched=[]
            for output in candidates:
                if source.suffix.lower()!=output.suffix.lower(): continue
                code,r=cli(pixelita,["compare","--json","--strict-size","--min-psnr",
                    "200" if source.suffix.lower()==".jpg" else "30","--min-ssim",".95",source,output])
                gain=100*(source.stat().st_size-output.stat().st_size)/source.stat().st_size
                if code==0 and gain >= (10 if task=="collision" else 1): matched.append(str(output))
            matches[str(source)]=matched
        # Unique output per source; collision fixtures are deliberately different.
        def assign(groups, used):
            if not groups:return True
            return any(assign(groups[1:],used|{v}) for v in groups[0] if v not in used)
        checks["allOutputsMeetPolicy"]=assign(list(matches.values()),set())
        checks["outputCount"]=len(candidates)==len(inputs)
    return checks


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("output",type=Path)
    p.add_argument("--repeat",type=int,default=2)
    p.add_argument("--tasks",nargs="+",choices=list(TASKS),default=list(TASKS))
    a=p.parse_args()
    if a.repeat<1:p.error("repeat must be positive")
    repo=Path(__file__).resolve().parents[1]
    out=a.output.resolve(); out.mkdir(parents=True,exist_ok=True)
    provenance={"cliVersion":subprocess.check_output(["codex","--version"],text=True).strip(),
        "binarySHA256":{p.name:sha(p) for p in sorted((repo/"bin").glob("*.exe"))},
        "skillSHA256":{p.relative_to(repo/".agents/skills/pixelita").as_posix():sha(p)
                       for p in sorted((repo/".agents/skills/pixelita").rglob("*")) if p.is_file()},
        "fixtureGeneratorSHA256":sha(repo/"eval/generate_corpus.py"),
        "harnessSHA256":sha(Path(__file__)),
        "model":"CLI default, not pinned", "sandbox":"workspace-write", "approval":"never"}
    manifest_path=out/"trial-manifest.json"
    if manifest_path.exists() and json.loads(manifest_path.read_text(encoding="utf-8"))!=provenance:
        p.error("resume requires unchanged binaries, skill, and runner configuration")
    manifest_path.write_text(json.dumps(provenance,indent=2)+"\n",encoding="utf-8")
    corpus=out/"fixtures"
    if not corpus.exists():
        subprocess.run([sys.executable,str(repo/"eval/generate_corpus.py"),str(corpus)],check=True)
    disabled=Path.home()/".agents/skills/pixelita/SKILL.md"
    disable_config='skills.config=[{path='+json.dumps(disabled.as_posix())+',enabled=false}]'
    for rep in range(a.repeat):
        for task in a.tasks:
            for guided in ([False,True] if rep%2==0 else [True,False]):
                label=f"{task}-{'guided' if guided else 'help'}-{rep+1}"
                result_path=out/(label+".json")
                if result_path.exists():
                    if json.loads(result_path.read_text(encoding="utf-8")).get("infrastructureBlocked"):
                        raise RuntimeError("Previous attempt was policy-blocked; use a fresh directory after the execution environment is resolved")
                    continue
                root=out/label
                if root.exists():raise RuntimeError("unfinished trial directory: "+str(root))
                root.mkdir()
                (root/"inputs").mkdir(); (root/"out").mkdir()
                shutil.copytree(repo/"bin",root/"bin")
                if guided:shutil.copytree(repo/".agents/skills/pixelita",root/"guide")
                if task=="optimize":
                    for name in ["flat-rgba.png","photo-rgb.png","baseline.jpg"]:shutil.copy2(corpus/name,root/"inputs"/name)
                elif task=="collision":
                    for side in ["left","right"]:shutil.copytree(corpus/side,root/"inputs"/side)
                    (root/"out/same-min.png").write_bytes(b"PREEXISTING DO NOT CHANGE\n")
                else:
                    shutil.copy2(corpus/"photo-rgb.png",root/"inputs/before.png")
                    im=Image.open(corpus/"photo-rgb.png").convert("RGB")
                    ImageDraw.Draw(im).rectangle((384,256,511,383),fill=(0,0,0))
                    im.save(root/"inputs/after.png")
                immutable=[p for p in (root/"inputs").rglob("*") if p.is_file()]
                if task=="collision":immutable.append(root/"out/same-min.png")
                hashes={p:sha(p) for p in immutable}
                prompt=("Work only inside this isolated workspace. Use the Pixelita binaries in ./bin (Windows .exe). "
                    "Do not read external files, use network services, delegate tasks, install anything, or edit the binaries. "
                    "Do not ask questions; complete the task using local executable help if needed. "
                    +("Read guide/SKILL.md and the relevant references first. " if guided else "")
                    +TASKS[task])
                (root/"prompt.txt").write_text(prompt,encoding="utf-8")
                argv=["codex","--no-daemon","-a","never","exec","--ignore-user-config","--ephemeral",
                    "--skip-git-repo-check","--json","--sandbox","workspace-write",
                    "-c",disable_config,"-c","features.apps=false","-C",str(root),"-"]
                started=time.perf_counter()
                print("START",label,flush=True)
                with (out/(label+".jsonl")).open("wb") as log,(out/(label+".stderr")).open("wb") as err:
                    proc=subprocess.Popen(argv,stdin=subprocess.PIPE,stdout=log,stderr=err)
                    timed_out=False
                    try:proc.communicate(prompt.encode(),timeout=240)
                    except subprocess.TimeoutExpired:
                        timed_out=True
                        if os.name == "nt":
                            subprocess.run(["taskkill", "/PID", str(proc.pid), "/T", "/F"], capture_output=True)
                        else:
                            proc.kill()
                        proc.communicate()
                agent_seconds=time.perf_counter()-started
                stderr=(out/(label+".stderr")).read_text(encoding="utf-8",errors="replace")
                rejected=stderr.count("rejected: blocked by policy")
                events=[]
                for line in (out/(label+".jsonl")).read_text(encoding="utf-8-sig").splitlines():
                    try:events.append(json.loads(line))
                    except ValueError:pass
                commands=[e["item"] for e in events if e.get("type")=="item.completed" and e.get("item",{}).get("type")=="command_execution"]
                checks=grade(root,task,hashes,repo/"bin/pixelita.exe")
                result={"task":task,"guided":guided,"repetition":rep+1,"exit":proc.returncode,
                    "timedOut":timed_out,"agentSeconds":agent_seconds,
                    "infrastructureBlocked":rejected>0,"rejectedLaunches":rejected,
                    "commandCalls":len(commands),"failedCommands":sum(c.get("exit_code",0)!=0 for c in commands),
                    "checks":checks,"passed":proc.returncode==0 and not timed_out and rejected==0 and all(checks.values()),
                    "usage":next((e.get("usage") for e in reversed(events) if e.get("type")=="turn.completed"),None)}
                result_path.write_text(json.dumps(result,indent=2)+"\n",encoding="utf-8")
                print("DONE",label,json.dumps(result),flush=True)
                if rejected:
                    raise RuntimeError("Nested runner blocked command execution; stopping trials without changing policy")


if __name__=="__main__":main()
