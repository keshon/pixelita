"""Evaluate stricter floors against saved PNG candidates without re-encoding."""
import argparse
import json
from pathlib import Path
from compare_png import accept

p=argparse.ArgumentParser(description=__doc__)
p.add_argument("directory",type=Path)
p.add_argument("--overrides",type=Path)
p.add_argument("--out",type=Path,required=True)
a=p.parse_args()
manifest=json.loads((a.directory/"manifest.json").read_text(encoding="utf-8"))
rows=[]
for original in manifest["items"]:
    if original["metrics"]["format"]!="png":
        continue
    record=a.directory/(original["id"]+".json")
    if a.overrides and (a.overrides/record.name).exists():
        record=a.overrides/record.name
    r=json.loads(record.read_text(encoding="utf-8"))
    item=r["pixelita"].get("report",{}).get("items",[{}])[0]
    rows.append((original,item))
out={"schema":1,"inputs":len(rows),"inputBytes":sum(i["bytesBefore"] for i,_ in rows),
     "measuredCandidates":sum(i.get("verification",{}).get("status")=="measured" for _,i in rows),
     "scenarios":[],"notes":["Re-evaluates saved candidates; does not search different encoder settings.",
     "Unsupported or unmeasured candidates retain the source.",
     "Higher thresholds are not a perceptual-losslessness guarantee.",
     "No default changes are implied by these local corpus measurements."]}
for psnr,ssim in [(30,.95),(35,.98),(40,.99)]:
    policy={"minGain":10,"minPSNR":psnr,"minSSIM":ssim}
    accepted=0
    effective=0
    for source,item in rows:
        measured=item.get("verification",{}).get("status")=="measured"
        passed=measured and accept(item["bytesAfter"],source["bytesBefore"],item["metrics"],policy)
        accepted+=int(passed)
        effective+=item["bytesAfter"] if passed else source["bytesBefore"]
    out["scenarios"].append({"policy":policy,"accepted":accepted,"effectiveBytes":effective,
                            "savedBytes":out["inputBytes"]-effective})
a.out.write_text(json.dumps(out,indent=2)+"\n",encoding="utf-8")
print(json.dumps(out,indent=2))
