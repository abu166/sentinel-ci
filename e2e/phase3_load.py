"""Bounded live health load with real Docker sandbox concurrency sampling.

Run while phase3.py executes. Container names establish observations only; runtime
isolation is checked from Docker HostConfig, not untrusted job-reported metrics.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import statistics
import subprocess
import threading
import time
import urllib.request
from simulate import BASE


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--seconds',type=int,default=90)
    parser.add_argument('--require-parallel',action='store_true')
    parser.add_argument('--require-inspection',action='store_true')
    parser.add_argument('--output',default='/tmp/sentinel-phase3-load.json')
    options=parser.parse_args()
    stop=threading.Event()
    latencies=[]
    failures=[]
    lock=threading.Lock()
    def health():
        while not stop.is_set():
            started=time.monotonic()
            try:
                with urllib.request.urlopen(BASE+'/api/v1/health',timeout=10) as response:
                    assert response.status==200 and json.load(response)['status']=='healthy'
                with lock: latencies.append(time.monotonic()-started)
            except Exception as error:
                with lock: failures.append(type(error).__name__)
            stop.wait(.1)
    peak=0
    samples=0
    observed=set()
    started=time.monotonic()
    with ThreadPoolExecutor(max_workers=8) as pool:
        workers=[pool.submit(health) for _ in range(8)]
        try:
            while time.monotonic()-started<options.seconds:
                result=subprocess.run(['docker','ps','--filter','name=sentinel-verify-','--format','{{.ID}}'],capture_output=True,text=True,check=True)
                ids=result.stdout.split()
                peak=max(peak,len(ids))
                samples+=1
                assert len(ids)<=3,'Global candidate sandbox concurrency exceeded3'
                for identity in ids:
                    if identity in observed: continue
                    inspect=subprocess.run(['docker','inspect',identity],capture_output=True,text=True)
                    if inspect.returncode: continue # --rm container finished between ps and inspect
                    host=json.loads(inspect.stdout)[0]['HostConfig']
                    assert host['ReadonlyRootfs'] and host['NetworkMode']=='none'
                    assert host['Memory']>0 and host['NanoCpus']>0 and 0<host['PidsLimit']<=128
                    assert 'ALL' in host['CapDrop'] and any('no-new-privileges' in x for x in host['SecurityOpt'])
                    observed.add(identity)
                stop.wait(.2)
        finally:
            stop.set()
            for worker in workers: worker.result()
    assert latencies and not failures, {'health_failures':failures,'successes':len(latencies)}
    if options.require_parallel: assert peak>=2,'No actual parallel candidate sandboxes observed'
    if options.require_inspection: assert observed, 'No live container HostConfig inspected'
    sorted_latency=sorted(latencies)
    report={'result':'passed','duration_seconds':round(time.monotonic()-started,2),'health_clients':8,'health_requests':len(latencies),'health_errors':len(failures),'p50_ms':round(statistics.median(latencies)*1000,2),'p95_ms':round(sorted_latency[int((len(sorted_latency)-1)*.95)]*1000,2),'docker_samples':samples,'peak_candidate_containers':peak,'isolated_containers_inspected':len(observed),'limits':'Local unthrottled load; peak sampling is observational, code/unit tests enforce global semaphore'}
    with open(options.output,'w') as handle: json.dump(report,handle,indent=2)
    print(json.dumps(report,indent=2))

if __name__=='__main__': main()
