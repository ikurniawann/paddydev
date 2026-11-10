"""Reference importer: Paddy SmartArt org chart (sheet All + sheet Retail) -> normalized position seed CSV.

Usage:
    mkdir src && cd src && unzip ../Org_Chart_Paddy_2026_-_Draft_6_1.xlsx && cd ..
    python3 paddy_org_smartart_import.py src paddy_org_positions_seed.csv

Notes:
- Title normalization (FIX, CTX_STAFF, NEEDS_CONFIRM) is hard-coded for Draft 6.
  Story ORG-06 moves these maps into a `title_aliases` table.
- The Retail Operational subtree of sheet All is replaced by sheet Retail (store-level detail).
"""
import sys, re, csv, json, xml.etree.ElementTree as ET
from collections import Counter, defaultdict
A='{http://schemas.openxmlformats.org/drawingml/2006/main}'
D='{http://schemas.openxmlformats.org/drawingml/2006/diagram}'
DS='{http://schemas.microsoft.com/office/drawing/2008/diagram}'
base=sys.argv[1]; out=sys.argv[2]

def load(i):
    r=ET.parse(f'{base}/xl/diagrams/data{i}.xml').getroot()
    pts={}
    for pt in r.find(D+'ptLst'):
        t=pt.get('type','node')
        tb=pt.find(D+'t')
        txt=' | '.join(''.join(x.text or '' for x in p.iter(A+'t')) for p in tb.findall(A+'p')) if tb is not None else ''
        pts[pt.get('modelId')]=(t,txt)
    kids=defaultdict(list)
    for c in r.find(D+'cxnLst'):
        if c.get('type','parOf')=='parOf': kids[c.get('srcId')].append((int(c.get('srcOrd','0')),c.get('destId')))
    # highlighted shapes (non-default fill) keyed by modelId of drawing shape
    dr=ET.parse(f'{base}/xl/diagrams/drawing{i}.xml').getroot()
    hl=set()
    for sp in dr.iter(DS+'sp'):
        sf=sp.find(DS+'spPr/'+A+'solidFill')
        if sf is not None and len(sf) and sf[0].get('val')=='accent5': hl.add(sp.get('modelId'))
    # map drawing shape modelId -> node via pres points
    r2=r
    pres2node={}
    for pt in r2.find(D+'ptLst'):
        pr=pt.find(D+'prSet')
        if pt.get('type')=='pres' and pr is not None and pr.get('presAssocID'): pres2node[pt.get('modelId')]=pr.get('presAssocID')
    hl_nodes={pres2node.get(m,m) for m in hl}
    doc=[k for k,(t,_) in pts.items() if t=='doc'][0]
    return pts,kids,doc,hl_nodes

FIX={ # raw (whitespace-collapsed) -> normalized title, issue tag
 'Inbond Coordinator':('Inbound Coordinator','TYPO'),
 'Outbond Coordinator':('Outbound Coordinator','TYPO'),
 'Logistic Corrdinator':('Logistics Coordinator','TYPO'),
 'Admin Logistic':('Logistics Admin','WORDING'),
 'Account Receiveable | Staff':('Account Receivable Staff','TYPO'),
 'Branding & Marketing Assisstant Manager':('Branding & Marketing Assistant Manager','TYPO'),
 'Revenue Assisstant Manager':('Revenue Assistant Manager','TYPO'),
 'Brand Patnership Supervisor':('Brand Partnership Supervisor','TYPO'),
 'Brand Patnership Staff':('Brand Partnership Staff','TYPO'),
 'KOL Specialst Staff':('KOL Specialist Staff','TYPO'),
 'Purchasing':('Purchasing Staff','FUNCTION_AS_TITLE'),
 'Vendor Management':('Vendor Management Staff','FUNCTION_AS_TITLE'),
 'Procurement Control':('Procurement Control Staff','FUNCTION_AS_TITLE'),
 'Quality Control':('Quality Control Staff','FUNCTION_AS_TITLE'),
 'Administrator':('SCM Administrator','GENERIC_TITLE'),
 'Inventory Management':('Inventory Management Staff','FUNCTION_AS_TITLE'),
 'Talent Acquisition':('Talent Acquisition Staff','FUNCTION_AS_TITLE'),
 'HR Ops & Com':('HR Operations & Com Staff','FUNCTION_AS_TITLE;AMBIGUOUS_ABBR'),
 'People Ops & Development':('People Operations & Development Staff','FUNCTION_AS_TITLE'),
 'Senior Staff':('Finance Senior Staff','GENERIC_TITLE'),
 'Inventory Control':('Inventory Control','DUPLICATE_CANDIDATE;ASSISTANT_NODE'),
}
CTX_STAFF={'Packing Coordinator':'Packing Staff','Production Coordinator':'Production Staff',
           'Internal Audit Supervisor':'Internal Audit Staff','Sample Maker Lead':'Sample Maker Staff'}
DIV={'Operations Manager':('OPS','Operations'),'Finance & Accounting Manager':('FIN','Finance & Accounting'),
     'HRGA Manager':('HRG','HRGA'),'Support':('SUP','Corporate Support'),'Product R&D Manager':('RND','Product R&D'),
     'Branding & Marketing Manager':('MKT','Branding & Marketing'),'Retail Operational':('RTL','Retail Operational'),
     'Retail Operational Manager':('RTL','Retail Operational')}
STORE_RE=re.compile(r'\b([A-Z]{4})\b')

NEEDS_CONFIRM={'Corporate Secretary':'L5','IT Developer':'L6','Product Designer':'L6','Product Developer':'L6',
 'Art Director':'L5','CRM & Revenue Specialist':'L5','Sales Event Specialist':'L6','Merchandiser Specialist':'L6',
 'Admin Store & Control':'L7','Inventory Control':'L6','Brand Creative Lead':'L5'}
def level(t, has_kids):
    tl=t.lower()
    if t in NEEDS_CONFIRM: return NEEDS_CONFIRM[t],'needs_confirm'
    if t=='CEO': return 'L0','keyword'
    if 'general manager' in tl: return 'L1','keyword'
    if 'assistant manager' in tl: return 'L3','keyword'
    if 'manager' in tl and 'management' not in tl: return 'L2','keyword'
    if 'supervisor' in tl: return 'L4','keyword'
    if any(k in tl for k in ['coordinator','lead','store leader']): return 'L5','keyword'
    if re.search(r'\bintern\b',tl): return 'L8','keyword'
    if 'senior staff' in tl: return 'L6','keyword'
    if 'staff' in tl: return 'L7','keyword'
    return 'L7','role_rule'

rows=[]; seq=Counter()
def emit(pts,kids,nid,parent_row,div,region,sheet,hl,parent_raw):
    t,raw=pts[nid]
    raw_c=re.sub(r'\s+',' ',raw).strip()
    flags=[]
    if raw!=raw.strip() or '  ' in raw: flags.append('WHITESPACE')
    if ' | ' in raw: flags.append('LINEBREAK_IN_TITLE')
    if raw_c in DIV: div=DIV[raw_c]
    norm=raw_c; emp='PERMANENT_OR_CONTRACT'; vendor=''; stores=''
    if raw_c in FIX: norm,tag=FIX[raw_c]; flags+=tag.split(';')
    elif raw_c=='Staff': norm=CTX_STAFF.get(parent_raw,'Staff'); flags.append('GENERIC_TITLE')
    if raw_c=='Intern':
        ctx={'Recon Staff':'Recon','Account Payable Staff':'Account Payable','People Ops & Development':'People Ops & Development','Social Media Specialist Staff':'Social Media','KOL Specialst Staff':'KOL'}.get(parent_raw,parent_raw)
        norm=f'{ctx} Intern'; emp='INTERN'; flags+=['EMPLOYMENT_TYPE_AS_NODE','GENERIC_TITLE']
    if raw_c=='Freelance':
        ctx={'Talent Acquisition':'Talent Acquisition','GA Staff':'GA'}[parent_raw]
        norm=f'{ctx} Freelancer'; emp='FREELANCE'; flags+=['EMPLOYMENT_TYPE_AS_NODE','GENERIC_TITLE']
    if raw_c in ('Freelance Designer','Host Live Freelance'):
        norm={'Freelance Designer':'Freelance Product Designer','Host Live Freelance':'Live Host Freelancer'}[raw_c]; emp='FREELANCE'; flags.append('EMPLOYMENT_TYPE_IN_TITLE')
    if raw_c.startswith('Security'):
        norm='Security'; emp='OUTSOURCE'; vendor='Red Guard'; flags+=['EMPLOYMENT_TYPE_IN_TITLE','VENDOR_IN_TITLE','TYPO']
    if raw_c.startswith('Store Supervisor - '):
        region=raw_c.split(' - ',1)[1]; norm='Store Supervisor'; flags.append('LOCATION_IN_TITLE')
    if sheet=='Retail':
        codes=STORE_RE.findall(raw_c.replace('PPVJShopkeeper','PPVJ Shopkeeper').replace('PPBAShopkeeper','PPBA Shopkeeper'))
        if codes:
            stores=';'.join(codes)
            role=STORE_RE.sub('',raw_c.replace('PPVJShopkeeper','Shopkeeper').replace('PPBAShopkeeper','Shopkeeper')).replace('&','').replace('|','').strip()
            role=re.sub(r'\s+',' ',role)
            norm=role; flags.append('STORE_CODE_IN_TITLE')
            if 'Shopkeeper' in raw_c and re.search(r'[A-Z]{4}Shopkeeper',raw_c): flags.append('WHITESPACE')
            if len(codes)>1: flags.append('MULTI_STORE')
    is_pos='Y'
    if raw_c in ('Support','Retail Operational'): is_pos='N'; flags.append('UNIT_AS_NODE')
    if t=='asst': flags.append('ASSISTANT_NODE') if 'ASSISTANT_NODE' not in flags else None
    has_kids=bool(kids.get(nid))
    lv,conf=level(norm,has_kids)
    if is_pos=='N': lv,conf='',''
    if nid in hl: flags.append('HIGHLIGHTED_NO_LEGEND')
    dcode=div[0] if div else 'EXE'
    seq[dcode]+=1
    code=f'{dcode}-{seq[dcode]:03d}'
    # parent resolution: skip non-position grouping nodes
    p=parent_row
    while p is not None and p['is_position']=='N': p=p['_parent']
    row=dict(position_code=code,parent_code=p['position_code'] if p else '',source_sheet=sheet,smartart_type=t,
             raw_title=raw,normalized_title=norm,is_position=is_pos,division_code=dcode,division=div[1] if div else 'Executive',
             job_level=lv,level_confidence=conf,employment_type=emp if is_pos=='Y' else '',vendor=vendor,
             region=region or '',store_codes=stores,flags=';'.join(dict.fromkeys(flags)),_parent=p,_raw=raw_c)
    if p and lv and p['job_level'] and lv<=p['job_level'] and 'L' in lv:
        row['flags']=';'.join(filter(None,[row['flags'],'LEVEL_PARITY_WITH_PARENT']))
    rows.append(row)
    for o,k in sorted(kids.get(nid,[])):
        emit(pts,kids,k,row,div,region,sheet,hl,raw_c)
    return row

pts1,k1,doc1,hl1=load(1); pts2,k2,doc2,hl2=load(2)
root=k1[doc1][0][1]
# Walk sheet All, but replace 'Retail Operational' subtree with sheet Retail (store-level detail)
def walk_all(nid,parent,div,parent_raw):
    t,raw=pts1[nid]; rc=re.sub(r'\s+',' ',raw).strip()
    if rc=='Retail Operational':
        r2root=k2[doc2][0][1]
        emit(pts2,k2,r2root,parent,DIV['Retail Operational Manager'],None,'Retail',hl2,parent_raw)
        return
    # emit node only (no recursion), then recurse manually
    saved=k1.get(nid); k1[nid]=[]
    row=emit(pts1,k1,nid,parent,DIV.get(rc,div),None,'All',hl1,parent_raw)
    k1[nid]=saved or []
    d=DIV.get(rc,div)
    for o,k in sorted(k1.get(nid,[])): walk_all(k,row,d,rc)
walk_all(root,None,None,'')

cols=[c for c in rows[0] if not c.startswith('_')]
with open(out,'w',newline='') as f:
    w=csv.DictWriter(f,fieldnames=cols); w.writeheader()
    for r in rows: w.writerow({c:r[c] for c in cols})
# stats
print('rows',len(rows),'positions',sum(r['is_position']=='Y' for r in rows))
print('by division',Counter(r['division'] for r in rows if r['is_position']=='Y'))
print('by level',sorted(Counter(r['job_level'] for r in rows if r['is_position']=='Y').items()))
print('by emp',Counter(r['employment_type'] for r in rows if r['is_position']=='Y'))
fc=Counter(f for r in rows for f in r['flags'].split(';') if f); print('flags',fc)
print('low conf',[(r['position_code'],r['normalized_title'],r['job_level']) for r in rows if r['level_confidence']=='needs_confirm'])
print('parity',[(r['normalized_title'],r['_parent']['normalized_title']) for r in rows if 'LEVEL_PARITY' in r['flags']])
print('highlighted',[(r['position_code'],r['normalized_title'],r['region']) for r in rows if 'HIGHLIGHTED' in r['flags']])
st=[c for r in rows for c in r['store_codes'].split(';') if c]; print('store codes',len(set(st)),sorted(set(st)))
withlead={c for r in rows if r['normalized_title']=='Store Leader' for c in r['store_codes'].split(';')}
print('stores with leader',len(withlead),'without',sorted(set(st)-withlead))
print('titles uniq',len({r['normalized_title'] for r in rows if r['is_position']=='Y'}))
dup=Counter(r['normalized_title'] for r in rows if r['is_position']=='Y'); print('dup titles',{k:v for k,v in dup.items() if v>1})
