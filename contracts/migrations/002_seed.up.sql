-- Deterministic portfolio demonstration history; not live execution telemetry.
INSERT INTO repositories(id,name,repo_url,default_branch) VALUES ('f6cff0f4-220e-596d-8ca4-342b6474c316','payment-api','https://github.com/example/payment-api','main') ON CONFLICT DO NOTHING;
INSERT INTO repositories(id,name,repo_url,default_branch) VALUES ('fa542be1-57f3-5fbb-afea-bbade7669814','inventory-worker','https://github.com/example/inventory-worker','main') ON CONFLICT DO NOTHING;
INSERT INTO repositories(id,name,repo_url,default_branch) VALUES ('6bb49970-0c92-5ca8-979a-dff53909f7d8','analytics-engine','https://github.com/example/analytics-engine','develop') ON CONFLICT DO NOTHING;
INSERT INTO repositories(id,name,repo_url,default_branch) VALUES ('b3e605e5-4cbe-5782-91f1-e1ff59229aea','identity-service','https://github.com/example/identity-service','main') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('6703c673-aade-57e8-ac64-3ee79a34eccf','f6cff0f4-220e-596d-8ca4-342b6474c316','6af613b6569c5c229c372ed93f31d3afa1b2c3d4','main','PASSED','seed-demo',now() - interval '2 hours',now() - interval '2 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('5c2968b6-4872-5cb4-b685-160634051e93','6703c673-aade-57e8-ac64-3ee79a34eccf','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5c2968b6-4872-5cb4-b685-160634051e93',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5c2968b6-4872-5cb4-b685-160634051e93',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5c2968b6-4872-5cb4-b685-160634051e93',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('2ac99777-8092-571e-bec4-aedcf7c1fa98','6703c673-aade-57e8-ac64-3ee79a34eccf','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2ac99777-8092-571e-bec4-aedcf7c1fa98',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2ac99777-8092-571e-bec4-aedcf7c1fa98',2,'stdout','Running go test ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2ac99777-8092-571e-bec4-aedcf7c1fa98',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('d6ed2ba9-b00a-542e-aa8a-bb143fe3a38c','6703c673-aade-57e8-ac64-3ee79a34eccf','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d6ed2ba9-b00a-542e-aa8a-bb143fe3a38c',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d6ed2ba9-b00a-542e-aa8a-bb143fe3a38c',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d6ed2ba9-b00a-542e-aa8a-bb143fe3a38c',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('2088d6d7-b237-5505-ada4-603ec849b8dc','6703c673-aade-57e8-ac64-3ee79a34eccf','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2088d6d7-b237-5505-ada4-603ec849b8dc',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2088d6d7-b237-5505-ada4-603ec849b8dc',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2088d6d7-b237-5505-ada4-603ec849b8dc',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('5ce6cc5c-224f-5245-8642-0198afbf6314','fa542be1-57f3-5fbb-afea-bbade7669814','b04965e6a9bb591f8f8a1adcb2c8dc39a1b2c3d4','main','HEALED','seed-demo',now() - interval '7 hours',now() - interval '7 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('c1b5da9f-73a0-5572-adbf-cd9d2d233455','5ce6cc5c-224f-5245-8642-0198afbf6314','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c1b5da9f-73a0-5572-adbf-cd9d2d233455',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c1b5da9f-73a0-5572-adbf-cd9d2d233455',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c1b5da9f-73a0-5572-adbf-cd9d2d233455',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('bab0cdb4-a169-53cc-93b8-19b0af762199','5ce6cc5c-224f-5245-8642-0198afbf6314','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bab0cdb4-a169-53cc-93b8-19b0af762199',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bab0cdb4-a169-53cc-93b8-19b0af762199',2,'stdout','Traceback (most recent call last):') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bab0cdb4-a169-53cc-93b8-19b0af762199',3,'stdout','  File "collection.py", line 2, in last_item') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bab0cdb4-a169-53cc-93b8-19b0af762199',4,'stdout','IndexError: list index out of range') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bab0cdb4-a169-53cc-93b8-19b0af762199',5,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('3e8a0ade-bf66-5e37-a112-fb2314ddf8a1','5ce6cc5c-224f-5245-8642-0198afbf6314','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('3e8a0ade-bf66-5e37-a112-fb2314ddf8a1',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('3e8a0ade-bf66-5e37-a112-fb2314ddf8a1',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('3e8a0ade-bf66-5e37-a112-fb2314ddf8a1',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('bcc3f61b-b43b-586e-a67f-ea17578d2981','5ce6cc5c-224f-5245-8642-0198afbf6314','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bcc3f61b-b43b-586e-a67f-ea17578d2981',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bcc3f61b-b43b-586e-a67f-ea17578d2981',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('bcc3f61b-b43b-586e-a67f-ea17578d2981',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('5ce6cc5c-224f-5245-8642-0198afbf6314','{"pipeline_id": "5ce6cc5c-224f-5245-8642-0198afbf6314", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"collection.py": "def last_item(items):\n    return items[len(items)]\n", "test_repair.py": "from collection import last_item\nimport unittest\nclass TestRepair(unittest.TestCase):\n    def test_boundary(self): self.assertEqual(last_item([1,2,3]),3)\n"}, "steps": [{"id": "bab0cdb4-a169-53cc-93b8-19b0af762199", "name": "Unit Tests", "stage": "test", "command": "python3 -m unittest discover -v", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('fb6f1d2f-c5e7-5695-82bb-89446dae318a','5ce6cc5c-224f-5245-8642-0198afbf6314','bab0cdb4-a169-53cc-93b8-19b0af762199','collection.py','boundary_off_by_one','The final valid list index is len(items) - 1.','Function scope; targeted mutation with regression verification','def last_item(items):
    return items[len(items)]
','def last_item(items):
    return items[len(items) - 1]
','--- a/collection.py
+++ b/collection.py
@@ -1,2 +1,2 @@
 def last_item(items):
-    return items[len(items)]
+    return items[len(items) - 1]
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('d6d5e064-c447-5e85-8bf4-4b4680510192','6bb49970-0c92-5ca8-979a-dff53909f7d8','4b166dbed99d5091abdd95b83330ed3aa1b2c3d4','develop','FAILED','seed-demo',now() - interval '12 hours',now() - interval '12 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('8751db25-137d-50a3-81a6-4b3b0edb8f93','d6d5e064-c447-5e85-8bf4-4b4680510192','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('8751db25-137d-50a3-81a6-4b3b0edb8f93',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('8751db25-137d-50a3-81a6-4b3b0edb8f93',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('8751db25-137d-50a3-81a6-4b3b0edb8f93',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('ccc75ceb-c749-5d0c-8bdd-790e4dc1a9fb','d6d5e064-c447-5e85-8bf4-4b4680510192','Unit Tests','unit-tests','go test ./...','FAILED',1,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('ccc75ceb-c749-5d0c-8bdd-790e4dc1a9fb',1,'stderr','panic: runtime error: invalid memory address or nil pointer dereference') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('ccc75ceb-c749-5d0c-8bdd-790e4dc1a9fb',2,'stderr','payment.go:42: processPayment') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('ccc75ceb-c749-5d0c-8bdd-790e4dc1a9fb',3,'stderr','FAIL: TestProcessPayment') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('4e443179-5614-583c-a930-cccb59d038d1','d6d5e064-c447-5e85-8bf4-4b4680510192','Integration','integration','go test -tags integration ./...','PENDING',NULL,NULL,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4e443179-5614-583c-a930-cccb59d038d1',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('2f548355-c028-594f-a159-5222ad55d34b','d6d5e064-c447-5e85-8bf4-4b4680510192','Verify Patch','verify-patch','go test -race ./...','PENDING',NULL,NULL,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2f548355-c028-594f-a159-5222ad55d34b',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('04921977-4ba1-5419-8c45-69c2cb99dc67','b3e605e5-4cbe-5782-91f1-e1ff59229aea','98123fde012f5ff38b50881449dac91aa1b2c3d4','main','RUNNING','seed-demo',now() - interval '17 hours',NULL) ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('9e955cfd-e084-5d12-8311-8e95fe4d9929','04921977-4ba1-5419-8c45-69c2cb99dc67','Lint','lint','go vet ./...','PENDING',NULL,NULL,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('9e955cfd-e084-5d12-8311-8e95fe4d9929',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('e3636ace-e264-5a51-8f3f-433a7fdbf034','04921977-4ba1-5419-8c45-69c2cb99dc67','Unit Tests','unit-tests','go test ./...','PENDING',NULL,NULL,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('e3636ace-e264-5a51-8f3f-433a7fdbf034',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('7fbbb5ec-cfe9-55a3-9b0e-ffa17b30232a','04921977-4ba1-5419-8c45-69c2cb99dc67','Integration','integration','go test -tags integration ./...','PENDING',NULL,NULL,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('7fbbb5ec-cfe9-55a3-9b0e-ffa17b30232a',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('c381ddee-a4f5-5b90-bb65-7425fb9b1ea5','04921977-4ba1-5419-8c45-69c2cb99dc67','Verify Patch','verify-patch','go test -race ./...','PENDING',NULL,NULL,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c381ddee-a4f5-5b90-bb65-7425fb9b1ea5',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('ce78ab95-779b-5214-b681-4e65b8013cae','f6cff0f4-220e-596d-8ca4-342b6474c316','6ed955c6506a53439be42c0afae02eefa1b2c3d4','main','HEALING','seed-demo',now() - interval '22 hours',NULL) ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('2da01691-7f1f-5872-9be2-2538d830a680','ce78ab95-779b-5214-b681-4e65b8013cae','Lint','lint','go vet ./...','PENDING',NULL,NULL,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2da01691-7f1f-5872-9be2-2538d830a680',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('7ececb9a-86d5-5394-9fed-19db759f97c2','ce78ab95-779b-5214-b681-4e65b8013cae','Unit Tests','unit-tests','go test ./...','PENDING',NULL,NULL,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('7ececb9a-86d5-5394-9fed-19db759f97c2',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('1a55551d-0c23-5b47-9a7e-b808cb14e89e','ce78ab95-779b-5214-b681-4e65b8013cae','Integration','integration','go test -tags integration ./...','PENDING',NULL,NULL,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('1a55551d-0c23-5b47-9a7e-b808cb14e89e',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('f63d38c2-5370-5e7e-b738-c78cb77fdf86','ce78ab95-779b-5214-b681-4e65b8013cae','Verify Patch','verify-patch','go test -race ./...','PENDING',NULL,NULL,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('f63d38c2-5370-5e7e-b738-c78cb77fdf86',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('3d8d333f-9e04-55f4-94be-e9f20c809837','fa542be1-57f3-5fbb-afea-bbade7669814','c8691da2158a5ed685370e6f140801f2a1b2c3d4','main','PENDING','seed-demo',NULL,NULL) ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('1294a36e-d989-5791-ac3b-165cade72000','3d8d333f-9e04-55f4-94be-e9f20c809837','Lint','lint','go vet ./...','PENDING',NULL,NULL,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('1294a36e-d989-5791-ac3b-165cade72000',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('7d1a6c1d-3470-5c7c-847e-67892e9c957e','3d8d333f-9e04-55f4-94be-e9f20c809837','Unit Tests','unit-tests','go test ./...','PENDING',NULL,NULL,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('7d1a6c1d-3470-5c7c-847e-67892e9c957e',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('4ae32500-40b2-57ba-8562-b5897f5df279','3d8d333f-9e04-55f4-94be-e9f20c809837','Integration','integration','go test -tags integration ./...','PENDING',NULL,NULL,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4ae32500-40b2-57ba-8562-b5897f5df279',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('18ec3162-4aee-58ed-b492-04852bdbdf91','3d8d333f-9e04-55f4-94be-e9f20c809837','Verify Patch','verify-patch','go test -race ./...','PENDING',NULL,NULL,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('18ec3162-4aee-58ed-b492-04852bdbdf91',1,'stdout','Step queued; awaiting dependencies') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('4755fb5b-227e-5d33-9b21-5704705b0186','6bb49970-0c92-5ca8-979a-dff53909f7d8','a6c4fc8f695051dea9ae2c519c465071a1b2c3d4','develop','HEALED','seed-demo',now() - interval '32 hours',now() - interval '32 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('4e767f34-1b78-57bc-8d3c-18bf2a2bcc50','4755fb5b-227e-5d33-9b21-5704705b0186','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4e767f34-1b78-57bc-8d3c-18bf2a2bcc50',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4e767f34-1b78-57bc-8d3c-18bf2a2bcc50',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4e767f34-1b78-57bc-8d3c-18bf2a2bcc50',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('d39caaa7-4253-5375-98d7-ef1af2415712','4755fb5b-227e-5d33-9b21-5704705b0186','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d39caaa7-4253-5375-98d7-ef1af2415712',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d39caaa7-4253-5375-98d7-ef1af2415712',2,'stdout','panic: runtime error: invalid memory address or nil pointer dereference') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d39caaa7-4253-5375-98d7-ef1af2415712',3,'stdout','payment.go:4 amount') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d39caaa7-4253-5375-98d7-ef1af2415712',4,'stdout','--- FAIL: TestAmount') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d39caaa7-4253-5375-98d7-ef1af2415712',5,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('e355c91d-a20c-50d8-9a24-8abee6d1aff5','4755fb5b-227e-5d33-9b21-5704705b0186','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('e355c91d-a20c-50d8-9a24-8abee6d1aff5',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('e355c91d-a20c-50d8-9a24-8abee6d1aff5',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('e355c91d-a20c-50d8-9a24-8abee6d1aff5',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('1aa34d8a-dfe9-578c-9e7f-0c0b9e756e3b','4755fb5b-227e-5d33-9b21-5704705b0186','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('1aa34d8a-dfe9-578c-9e7f-0c0b9e756e3b',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('1aa34d8a-dfe9-578c-9e7f-0c0b9e756e3b',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('1aa34d8a-dfe9-578c-9e7f-0c0b9e756e3b',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('4755fb5b-227e-5d33-9b21-5704705b0186','{"pipeline_id": "4755fb5b-227e-5d33-9b21-5704705b0186", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"payment.go": "package demo\ntype Payment struct { Amount int }\nfunc amount(p *Payment) int {\n    return p.Amount\n}\n", "repair_test.go": "package demo\nimport \"testing\"\nfunc TestAmount(t *testing.T) { if amount(nil)!=0 { t.Fatal(\"nil amount\") } }\n", "go.mod": "module demo\ngo 1.23\n"}, "steps": [{"id": "d39caaa7-4253-5375-98d7-ef1af2415712", "name": "Unit Tests", "stage": "test", "command": "go test ./...", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('0762bc2a-62d5-5476-a608-195c12c6f8cb','4755fb5b-227e-5d33-9b21-5704705b0186','d39caaa7-4253-5375-98d7-ef1af2415712','payment.go','nil_guard','A nil payment pointer reaches the amount access.','Function scope; targeted mutation with regression verification','package demo
type Payment struct { Amount int }
func amount(p *Payment) int {
    return p.Amount
}
','package demo
type Payment struct { Amount int }
func amount(p *Payment) int {
    if p == nil { return 0 }
    return p.Amount
}
','--- a/payment.go
+++ b/payment.go
@@ -1,5 +1,6 @@
 package demo
 type Payment struct { Amount int }
 func amount(p *Payment) int {
+    if p == nil { return 0 }
     return p.Amount
 }
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('a5fe086f-f8b4-5e4b-9564-cbab7edee0f8','b3e605e5-4cbe-5782-91f1-e1ff59229aea','a9f96b98dd445216ab0ddbfc6b262edfa1b2c3d4','main','PASSED','seed-demo',now() - interval '37 hours',now() - interval '37 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('20ab1b08-30cf-5b72-b768-cffbd6002fc5','a5fe086f-f8b4-5e4b-9564-cbab7edee0f8','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('20ab1b08-30cf-5b72-b768-cffbd6002fc5',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('20ab1b08-30cf-5b72-b768-cffbd6002fc5',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('20ab1b08-30cf-5b72-b768-cffbd6002fc5',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('9eafb4de-6f78-5625-aa80-d20e12ee8b73','a5fe086f-f8b4-5e4b-9564-cbab7edee0f8','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('9eafb4de-6f78-5625-aa80-d20e12ee8b73',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('9eafb4de-6f78-5625-aa80-d20e12ee8b73',2,'stdout','Running go test ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('9eafb4de-6f78-5625-aa80-d20e12ee8b73',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('24817e29-8da0-5100-ab18-070549acd9d4','a5fe086f-f8b4-5e4b-9564-cbab7edee0f8','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('24817e29-8da0-5100-ab18-070549acd9d4',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('24817e29-8da0-5100-ab18-070549acd9d4',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('24817e29-8da0-5100-ab18-070549acd9d4',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('d2d70e61-5d1c-57c9-b0ff-fb4f09e25f7e','a5fe086f-f8b4-5e4b-9564-cbab7edee0f8','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d2d70e61-5d1c-57c9-b0ff-fb4f09e25f7e',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d2d70e61-5d1c-57c9-b0ff-fb4f09e25f7e',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('d2d70e61-5d1c-57c9-b0ff-fb4f09e25f7e',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('d0b90eb0-63ad-51e1-9418-e558dbfa1119','f6cff0f4-220e-596d-8ca4-342b6474c316','e99caacd6c455906bd9fb79e62f25963a1b2c3d4','main','HEALED','seed-demo',now() - interval '42 hours',now() - interval '42 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('78dd6fc4-a07b-5c2a-bc26-66ace6732aad','d0b90eb0-63ad-51e1-9418-e558dbfa1119','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('78dd6fc4-a07b-5c2a-bc26-66ace6732aad',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('78dd6fc4-a07b-5c2a-bc26-66ace6732aad',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('78dd6fc4-a07b-5c2a-bc26-66ace6732aad',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e','d0b90eb0-63ad-51e1-9418-e558dbfa1119','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',2,'stdout','WARNING: DATA RACE') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',3,'stdout','Write at 0x00c000012180 by goroutine 8:') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',4,'stdout','counter.go:5 Counter.Inc') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',5,'stdout','--- FAIL: TestCounter (race detected during execution of test)') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e',6,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('6a8e1c99-8dcb-54e8-bf83-0b29d9e3ca27','d0b90eb0-63ad-51e1-9418-e558dbfa1119','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('6a8e1c99-8dcb-54e8-bf83-0b29d9e3ca27',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('6a8e1c99-8dcb-54e8-bf83-0b29d9e3ca27',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('6a8e1c99-8dcb-54e8-bf83-0b29d9e3ca27',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('4c244610-5395-5426-88a3-e360ffdb242a','d0b90eb0-63ad-51e1-9418-e558dbfa1119','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4c244610-5395-5426-88a3-e360ffdb242a',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4c244610-5395-5426-88a3-e360ffdb242a',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4c244610-5395-5426-88a3-e360ffdb242a',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('d0b90eb0-63ad-51e1-9418-e558dbfa1119','{"pipeline_id": "d0b90eb0-63ad-51e1-9418-e558dbfa1119", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"counter.go": "package demo\nimport \"sync\"\ntype Counter struct { mu sync.Mutex; value int }\nfunc (c *Counter) Inc() {\n    c.value++\n}\n", "repair_test.go": "package demo\nimport (\"testing\";\"sync\")\nfunc TestCounter(t *testing.T) { var c Counter; var wg sync.WaitGroup; for i:=0;i<20;i++ {wg.Add(1);go func(){defer wg.Done();for j:=0;j<100;j++{c.Inc()}}()}; wg.Wait();if c.value!=2000 {t.Fatal(c.value)} }\n", "go.mod": "module demo\ngo 1.23\n"}, "steps": [{"id": "5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e", "name": "Unit Tests", "stage": "test", "command": "go test ./...", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('5edab77a-71bc-5183-8fd8-6a89b2af2e77','d0b90eb0-63ad-51e1-9418-e558dbfa1119','5d4d4cc2-f0ad-5d2f-84fb-e11962675c5e','counter.go','missing_mutex','Concurrent map mutation requires locking.','Function scope; targeted mutation with regression verification','package demo
import "sync"
type Counter struct { mu sync.Mutex; value int }
func (c *Counter) Inc() {
    c.value++
}
','package demo
import "sync"
type Counter struct { mu sync.Mutex; value int }
func (c *Counter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
}
','--- a/counter.go
+++ b/counter.go
@@ -2,5 +2,7 @@
 import "sync"
 type Counter struct { mu sync.Mutex; value int }
 func (c *Counter) Inc() {
+    c.mu.Lock()
+    defer c.mu.Unlock()
     c.value++
 }
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('f0b43d0b-4715-53dd-9026-aad51489b727','fa542be1-57f3-5fbb-afea-bbade7669814','e4d80b30151e51b59f4f18a3b82718e6a1b2c3d4','main','HEALED','seed-demo',now() - interval '47 hours',now() - interval '47 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('48767930-150a-5164-952d-22c4bc69b723','f0b43d0b-4715-53dd-9026-aad51489b727','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('48767930-150a-5164-952d-22c4bc69b723',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('48767930-150a-5164-952d-22c4bc69b723',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('48767930-150a-5164-952d-22c4bc69b723',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('c7adb4eb-b923-5230-a193-5e2e7243d87e','f0b43d0b-4715-53dd-9026-aad51489b727','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c7adb4eb-b923-5230-a193-5e2e7243d87e',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c7adb4eb-b923-5230-a193-5e2e7243d87e',2,'stdout','Traceback (most recent call last):') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c7adb4eb-b923-5230-a193-5e2e7243d87e',3,'stdout','  File "stats_helper.py", line 2, in mean') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c7adb4eb-b923-5230-a193-5e2e7243d87e',4,'stdout','NameError: name ''statistics'' is not defined') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c7adb4eb-b923-5230-a193-5e2e7243d87e',5,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('c358fa95-7cef-5939-9da8-8e0a53c65473','f0b43d0b-4715-53dd-9026-aad51489b727','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c358fa95-7cef-5939-9da8-8e0a53c65473',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c358fa95-7cef-5939-9da8-8e0a53c65473',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('c358fa95-7cef-5939-9da8-8e0a53c65473',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('2fcf0157-ead6-5f12-9e4e-95623ef7c783','f0b43d0b-4715-53dd-9026-aad51489b727','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2fcf0157-ead6-5f12-9e4e-95623ef7c783',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2fcf0157-ead6-5f12-9e4e-95623ef7c783',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('2fcf0157-ead6-5f12-9e4e-95623ef7c783',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('f0b43d0b-4715-53dd-9026-aad51489b727','{"pipeline_id": "f0b43d0b-4715-53dd-9026-aad51489b727", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"stats_helper.py": "def mean(items):\n    return statistics.mean(items)\n", "test_repair.py": "from stats_helper import mean\nimport unittest\nclass TestRepair(unittest.TestCase):\n    def test_mean(self): self.assertEqual(mean([1,3]),2)\n"}, "steps": [{"id": "c7adb4eb-b923-5230-a193-5e2e7243d87e", "name": "Unit Tests", "stage": "test", "command": "python3 -m unittest discover -v", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('cdb6d979-5bfb-56b9-a94c-fbade4b0e20e','f0b43d0b-4715-53dd-9026-aad51489b727','c7adb4eb-b923-5230-a193-5e2e7243d87e','stats_helper.py','missing_import','The mean helper uses an unbound module name.','Function scope; targeted mutation with regression verification','def mean(items):
    return statistics.mean(items)
','import statistics
def mean(items):
    return statistics.mean(items)
','--- a/stats_helper.py
+++ b/stats_helper.py
@@ -1,2 +1,3 @@
+import statistics
 def mean(items):
     return statistics.mean(items)
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('9530e971-29c9-5bcd-bf21-001ded753f07','6bb49970-0c92-5ca8-979a-dff53909f7d8','0159d6c7973f5e7aa9a0d195d0ea6fe2a1b2c3d4','develop','HEALED','seed-demo',now() - interval '52 hours',now() - interval '52 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('363c3d6b-d21a-5604-ada1-bdee53882702','9530e971-29c9-5bcd-bf21-001ded753f07','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('363c3d6b-d21a-5604-ada1-bdee53882702',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('363c3d6b-d21a-5604-ada1-bdee53882702',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('363c3d6b-d21a-5604-ada1-bdee53882702',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('cf3f32aa-beab-5ed0-9735-797154b45835','9530e971-29c9-5bcd-bf21-001ded753f07','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('cf3f32aa-beab-5ed0-9735-797154b45835',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('cf3f32aa-beab-5ed0-9735-797154b45835',2,'stdout','Traceback (most recent call last):') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('cf3f32aa-beab-5ed0-9735-797154b45835',3,'stdout','  File "profile.py", line 2, in username') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('cf3f32aa-beab-5ed0-9735-797154b45835',4,'stdout','AttributeError: ''NoneType'' object has no attribute ''name''') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('cf3f32aa-beab-5ed0-9735-797154b45835',5,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('19f6a91c-8e71-5187-bbf1-d3d214a77cb6','9530e971-29c9-5bcd-bf21-001ded753f07','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('19f6a91c-8e71-5187-bbf1-d3d214a77cb6',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('19f6a91c-8e71-5187-bbf1-d3d214a77cb6',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('19f6a91c-8e71-5187-bbf1-d3d214a77cb6',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('91bb5ae8-c65b-5a81-865c-d371ae729bd9','9530e971-29c9-5bcd-bf21-001ded753f07','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('91bb5ae8-c65b-5a81-865c-d371ae729bd9',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('91bb5ae8-c65b-5a81-865c-d371ae729bd9',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('91bb5ae8-c65b-5a81-865c-d371ae729bd9',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('9530e971-29c9-5bcd-bf21-001ded753f07','{"pipeline_id": "9530e971-29c9-5bcd-bf21-001ded753f07", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"profile.py": "def username(profile):\n    return profile.name\n", "test_repair.py": "from profile import username\nimport unittest\nclass TestRepair(unittest.TestCase):\n    def test_none(self): self.assertIsNone(username(None))\n"}, "steps": [{"id": "cf3f32aa-beab-5ed0-9735-797154b45835", "name": "Unit Tests", "stage": "test", "command": "python3 -m unittest discover -v", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('e774986c-8158-5ceb-827c-18ca1533c3f6','9530e971-29c9-5bcd-bf21-001ded753f07','cf3f32aa-beab-5ed0-9735-797154b45835','profile.py','none_guard','An optional profile is dereferenced without checking None.','Function scope; targeted mutation with regression verification','def username(profile):
    return profile.name
','def username(profile):
    if profile is None:
        return None
    return profile.name
','--- a/profile.py
+++ b/profile.py
@@ -1,2 +1,4 @@
 def username(profile):
+    if profile is None:
+        return None
     return profile.name
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
INSERT INTO pipelines VALUES ('76d27b68-e666-5f9d-9aea-22c28c888af6','b3e605e5-4cbe-5782-91f1-e1ff59229aea','7fef88f7411d5669b42dbf5fc7f9b58ba1b2c3d4','main','HEALED','seed-demo',now() - interval '57 hours',now() - interval '57 hours' + interval '93 seconds') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('61fdb30f-aec4-5c8d-8712-a88525dd42e6','76d27b68-e666-5f9d-9aea-22c28c888af6','Lint','lint','go vet ./...','PASSED',0,1200,'{}') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('61fdb30f-aec4-5c8d-8712-a88525dd42e6',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('61fdb30f-aec4-5c8d-8712-a88525dd42e6',2,'stdout','Running go vet ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('61fdb30f-aec4-5c8d-8712-a88525dd42e6',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b','76d27b68-e666-5f9d-9aea-22c28c888af6','Unit Tests','unit-tests','go test ./...','PASSED',0,1510,ARRAY['Lint']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b',1,'stdout','Running regression suite') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b',2,'stdout','--- FAIL: TestLoad') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b',3,'stdout','load_test.go:3: missing error') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b',4,'stdout','FAIL demo') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b',5,'stdout','Deterministic repair generated; verification suite green') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('4cd9670d-bcad-585a-8d47-208ba9bd2c38','76d27b68-e666-5f9d-9aea-22c28c888af6','Integration','integration','go test -tags integration ./...','PASSED',0,1820,ARRAY['Unit Tests']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4cd9670d-bcad-585a-8d47-208ba9bd2c38',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4cd9670d-bcad-585a-8d47-208ba9bd2c38',2,'stdout','Running go test -tags integration ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('4cd9670d-bcad-585a-8d47-208ba9bd2c38',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO pipeline_steps VALUES ('64c04c45-0b45-5d42-9ef1-0d3f164bdd98','76d27b68-e666-5f9d-9aea-22c28c888af6','Verify Patch','verify-patch','go test -race ./...','PASSED',0,2130,ARRAY['Integration']) ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('64c04c45-0b45-5d42-9ef1-0d3f164bdd98',1,'stdout','Preparing isolated workspace') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('64c04c45-0b45-5d42-9ef1-0d3f164bdd98',2,'stdout','Running go test -race ./...') ON CONFLICT DO NOTHING;
INSERT INTO log_chunks(step_id,sequence_num,stream,content) VALUES('64c04c45-0b45-5d42-9ef1-0d3f164bdd98',3,'stdout','ok  sentinel/package  0.142s') ON CONFLICT DO NOTHING;
INSERT INTO job_snapshots(pipeline_id,payload) VALUES('76d27b68-e666-5f9d-9aea-22c28c888af6','{"pipeline_id": "76d27b68-e666-5f9d-9aea-22c28c888af6", "repo_url": "", "commit_sha": "demo", "branch": "seed/repair", "files": {"load.go": "package demo\nimport \"os\"\nfunc load(path string) ([]byte, error) {\n    data, err := os.ReadFile(path)\n    return data, nil\n}\n", "repair_test.go": "package demo\nimport \"testing\"\nfunc TestLoad(t *testing.T) { _,err:=load(\"/missing-file-sentinel\");if err==nil {t.Fatal(\"missing error\")} }\n", "go.mod": "module demo\ngo 1.23\n"}, "steps": [{"id": "5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b", "name": "Unit Tests", "stage": "test", "command": "go test ./...", "depends_on": []}]}'::jsonb) ON CONFLICT DO NOTHING;
INSERT INTO healing_patches(id,pipeline_id,step_id,file_path,bug_category,root_cause_analysis,ast_node_summary,original_code,patched_code,unified_diff,verification_status,confidence_score,test_output) VALUES ('c85c7c5f-8f62-5d32-b4d0-a9609597b671','76d27b68-e666-5f9d-9aea-22c28c888af6','5e2f0ced-08c7-5f73-b8ba-b8f9ebd62c6b','load.go','unhandled_error','Read failures must be returned before consuming data.','Function scope; targeted mutation with regression verification','package demo
import "os"
func load(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    return data, nil
}
','package demo
import "os"
func load(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil { return nil, err }
    return data, nil
}
','--- a/load.go
+++ b/load.go
@@ -2,5 +2,6 @@
 import "os"
 func load(path string) ([]byte, error) {
     data, err := os.ReadFile(path)
+    if err != nil { return nil, err }
     return data, nil
 }
','VERIFIED',0.97,'Demo history: 8 tests passed') ON CONFLICT DO NOTHING;
