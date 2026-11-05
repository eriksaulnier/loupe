import re, sys
W = 124
E = "\x1b["
def rgb(h): h=h.lstrip('#'); return tuple(int(h[i:i+2],16) for i in (0,2,4))
def fg(h): return E+"38;2;%d;%d;%dm"%rgb(h)
def bg(h): return E+"48;2;%d;%d;%dm"%rgb(h)
R=E+"0m"; B=E+"1m"; U=E+"4m"
BASE="#1e1e2e"; TEXT="#cdd6f4"; S0="#313244"; S1="#45475a"; ACC="#b4befe"; BRAND="#edb447"; APPLY="#94e2d5"
SEC="#a6adc8"; OV="#7f849c"; GOOD="#a6e3a1"; NOTE="#f9e2af"; RED="#f38ba8"; MAJOR="#ffa657"; MINOR="#e3b341"
def vis(s): return len(re.sub(r"\x1b\[[0-9;]*m","",s))
def pad(s,w): return s+" "*max(0,w-vis(s))
def t(s,c=TEXT): return fg(c)+s+R
def chip(s,fill,ink=BASE): return bg(fill)+fg(ink)+" "+s+" "+R
def bold(s,c=TEXT): return B+fg(c)+s+R
out=[]
def line(s=""): out.append(s)
def header(local, w=W, ref=None, title="", decided="0/4", gap=True):
    left = chip("⌕ loupe",BRAND,BASE)+" "
    if local:
        left += bg(S0)+" "+R+chip(local,APPLY)+bg(S0)+" "+fg(TEXT)+"widgets"+fg(OV)+" @ "+fg(TEXT)+"feat/cache"+fg(OV)+" · 2 files"+"   "+fg(SEC)+title+R
    else:
        left += bg(S0)+" "+B+U+fg(TEXT)+ref+R+bg(S0)+fg(OV)+"  round 1  "+fg(SEC)+title+R
    right = bg(S0)+fg(ACC)+"▮"+fg(S1)+"▮▮▮ "+B+fg(TEXT)+decided+" decided "+R
    fill = w-vis(left)-vis(right)
    line(left+bg(S0)+" "*max(1,fill)+R+right)
    if gap: line("")
def box_top(title_parts, w, c=ACC, right=""):
    s=fg(c)+"╭─ "+R+title_parts+fg(c)+" "
    rest=w-vis(s)-1-vis(right)-(2 if right else 0)
    s+="─"*rest+(" "+right+fg(c)+" " if right else "")+"╮"+R
    return s
def box_row(content,w,c=ACC): return fg(c)+"│"+R+" "+pad(content,w-4)+" "+fg(c)+"│"+R
def box_bot(w,c=ACC,right=""):
    if right: return fg(c)+"╰"+"─"*(w-vis(right)-5)+" "+R+right+fg(c)+" ─╯"+R
    return fg(c)+"╰"+"─"*(w-2)+"╯"+R
def bar(text,tone,indent=""): return indent+fg(tone)+"┃"+R+" "+text
def section(head,tone,body_lines,chips=""):
    rows=[bold(head,tone)+(" "+chips if chips else "")]
    rows+= [bar(b,tone) for b in body_lines]
    return rows
def code_pane(path,lines,hl,w,meta=""):
    rows=[box_top(fg(SEC)+path[0]+B+fg(TEXT)+path[1]+R, w, c="#8a90b8", right=meta)]
    for n,txt,mark in lines:
        num=fg(OV)+"%4d"%n+R
        if n in hl:
            rows.append(fg("#8a90b8")+"│"+R+" "+fg(ACC)+"▌"+bg(S0)+pad(num+bg(S0)+" "+(fg(GOOD)+"+ " if mark else "  ")+fg(TEXT)+txt, w-5)+R+" "+fg("#8a90b8")+"│"+R)
        else:
            rows.append(box_row(" "+num+"   "+fg(TEXT)+txt+R,w,c="#8a90b8"))
    rows.append(box_bot(w,c="#8a90b8"))
    return rows
def finding(fid,state,title,sev,sevc,meta_extra,blocks,pos,w=W,extra_chip=""):
    inner=[]
    inner.append(bold(title))
    inner.append((chip(sev,"#4a3b2a" if sev=="MAJOR" else "#3f3a2a",sevc) if sev else "")+meta_extra)
    for b in blocks:
        inner.append("")
        inner+= b
    tt=fg(ACC)+"· "+R+chip(fid,ACC)+" "+chip(state,S1,TEXT)+(" "+extra_chip if extra_chip else "")
    line(box_top(tt,w))
    for r in inner: line(box_row(r,w))
    while len(out)<34: line(box_row("",w))
    line(box_bot(w,right=fg(TEXT)+pos))
def footer(keys):
    line(" ".join((fg(OV)+"│"+R) if k=="|" else (B+fg(ACC)+k+R+" "+fg(TEXT)+d+R) for k,d in keys))
def wrap(s,n):
    words=s.split(); rows=[]; cur=""
    for wd in words:
        if len(cur)+len(wd)+1>n: rows.append(cur); cur=wd
        else: cur=(cur+" "+wd).strip()
    rows.append(cur); return rows
IW=W-6
plan=[(9,"Keep entries in a map guarded by one lock.",0),(10,"Evict the oldest entry once the map holds 5,000 entries.",0),(11,"",0),(12,"## Expiry",0),(13,"",0),(14,"An entry with an ETag is revalidated with a conditional request after one minute.",0),(15,"An entry without an ETag never expires.",0),(16,"",0),(17,"## Rollout",0)]
go=[(37,"func (m *Metrics) Collect(ch chan<- prometheus.Metric) {",0),(38,"    hits, misses := m.hits.Load(), m.misses.Load()",0),(39,"    m.hits.Store(0)",1),(40,"    m.misses.Store(0)",1),(41,"    ch <- prometheus.MustNewConstMetric(m.ratio, prometheus.GaugeValue, ratio(hits, misses))",0),(42,"}",0)]
lbody=wrap("The plan keeps an entry with no ETag forever, so a repository that sends none shows a stale title and head until the process restarts.",IW-4)
pbody=wrap("Collect stores zero into both counters after reading them. A second scraper, or a retried scrape, computes the ratio from a fraction of the traffic.",IW-4)
will=section("Claude will",GOOD,["Give every entry the one-minute TTL, and revalidate only the entries that carry an ETag."])
fix=section("Suggested fix",GOOD,["Export hits and misses as counters and let the dashboard compute the ratio."])
note=[bold("Note n-001",NOTE)+" "+chip("open",S0,NOTE),
      bar(chip("You",S1,TEXT),NOTE), bar("  Is the reset intentional? We only run one Prometheus.",NOTE),
      bar(chip("Agent",S1,TEXT)+fg(OV)+" · r-001"+R,NOTE), bar("  Kept as a suggestion: a retried scrape also halves the ratio, not only a second scraper.",NOTE)]
def why(rows): return [bold("Why",ACC)]+[bar(t(r),ACC) for r in rows]
lkeys=[("a","accept"),("A","tweak"),("x","reject"),("X","reject with reason"),("s","ask"),("|",""),("←→","finding"),("↑↓","scroll"),("f","file"),("l","list"),("?","help")]
pkeys=[("a","accept"),("x","exclude"),("s","send back"),("e","edit"),("w","rewrite"),("r","resolve"),("d","dismiss"),("|",""),("←→","finding"),("o","open PR"),("?","help")]
mode=sys.argv[1]
if mode=="local-A":
    header("local · returns to Claude",title="Cache plan")
    finding("f-001","pending","Entries without an ETag never expire","MAJOR",MAJOR,"",[code_pane(("docs/","cache-plan.md:12-16"),plan,{12,13,14,15,16},IW), why(lbody), will],"1 of 4")
    footer(lkeys)
elif mode=="local-C":
    header("local · returns to Claude",title="Cache plan")
    finding("f-001","pending","Entries without an ETag never expire","MAJOR",MAJOR,"",[will, code_pane(("docs/","cache-plan.md:12-16"),plan,{12,13,14,15,16},IW), why(lbody)],"1 of 4")
    footer(lkeys)
elif mode=="pr-A":
    header(None,ref="acme/widgets#42",title="feat(cache): add a read-through cache for pull request metadata",decided="3/6")
    finding("f-003","pending","Hit ratio counters reset on every scrape","MINOR",MINOR,"  "+chip("suggestion",S0,SEC)+"  "+t("confidence ",SEC)+fg(ACC)+"▰▰▱"+R+t("  plausible",SEC),[code_pane(("internal/cache/","metrics.go:39-40"),go,{39,40},IW,meta=fg(GOOD)+"+2 "+fg(RED)+"-0"+R), why(pbody), fix, note],"2 of 7",extra_chip=chip("open note",NOTE))
    footer(pkeys)
elif mode=="pr-C":
    header(None,ref="acme/widgets#42",title="feat(cache): add a read-through cache for pull request metadata",decided="3/6")
    finding("f-003","pending","Hit ratio counters reset on every scrape","MINOR",MINOR,"  "+chip("suggestion",S0,SEC)+"  "+t("confidence ",SEC)+fg(ACC)+"▰▰▱"+R+t("  plausible",SEC),[fix, code_pane(("internal/cache/","metrics.go:39-40"),go,{39,40},IW,meta=fg(GOOD)+"+2 "+fg(RED)+"-0"+R), why(pbody), note],"2 of 7",extra_chip=chip("open note",NOTE))
    footer(pkeys)
elif mode=="local-question":
    tone=NOTE
    qbody=wrap("Nothing in the plan clears an entry when the head moves. The choices are a webhook that deletes the entry, a shorter TTL, or comparing the head SHA on every read.",IW-4)
    ask=[bold("A question for Claude",tone)]+[bar(t("No change is proposed. Your note tells Claude what to do."),tone)]
    header("local · returns to Claude",title="Cache plan",decided="0/4")
    finding("f-004","pending","Who invalidates an entry on a force-push?","general",SEC,"",[why(qbody), ask],"4 of 4")
    footer(lkeys)
elif mode=="pr-withdrawn":
    mw=[(10,"flags := flag.NewFlagSet(\"widgets\", flag.ExitOnError)",0),(11,"size := flags.Int(\"cache-size\", 5000, \"entries to keep\")",1),(12,"ttl := flags.Duration(\"cache-ttl\", time.Minute, \"how long an entry stays fresh\")",0)]
    wd=[bold("Withdrawn",OV)+" "+chip("no-runtime-effect",S0,OV), bar(t("Both values agree, so nothing runs differently.",SEC),OV)]
    wy=[bold("Why",OV)]+[bar(t("The cache size default is set here and in the config file.",SEC),OV)]
    header(None,ref="acme/widgets#42",title="feat(cache): add a read-through cache for pull request metadata",decided="3/6")
    finding("f-007","withdrawn","Flag default duplicates the config file value","",SEC,chip("issue",S0,SEC),[wd, code_pane(("cmd/widgets/","main.go:11"),mw,{11},IW,meta=fg(GOOD)+"+1 "+fg(RED)+"-1"+R), wy],"7 of 7")
    footer([("u","reinstate"),("|",""),("←→","finding"),("o","open PR"),("?","help")])
elif mode=="headers":
    for w in (124,100,80):
        line(t("%d columns"%w,OV))
        if w==124: header("local · returns to Claude",w=w,title="Cache plan",gap=False)
        elif w==100:
            l=chip("⌕ loupe",BRAND)+" "+bg(S0)+" "+R+chip("local · Claude",APPLY)+bg(S0)+" "+fg(TEXT)+"widgets"+fg(OV)+" @ "+fg(TEXT)+"feat/cache"+"   "+fg(SEC)+"Cache plan"+R
            r=bg(S0)+fg(ACC)+"▮"+fg(S1)+"▮▮▮ "+B+fg(TEXT)+"0/4 decided "+R; line(l+bg(S0)+" "*(w-vis(l)-vis(r))+R+r)
        else:
            l=chip("⌕ loupe",BRAND)+" "+bg(S0)+" "+R+chip("local",APPLY)+bg(S0)+" "+fg(TEXT)+"feat/cache"+"   "+fg(SEC)+"Cache plan"+R
            r=bg(S0)+B+fg(TEXT)+" 0/4 decided "+R; line(l+bg(S0)+" "*(w-vis(l)-vis(r))+R+r)
        line("")
sys.stdout.write("\n".join(out)+"\n")
