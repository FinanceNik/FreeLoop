// Writes the made-up long session (frontend/src/longfixture.ts) as a transcript:
//   node scripts/appcheck/longsession.ts <shape> <sid> <events.jsonl>
import { writeFileSync } from "node:fs";
import { jsonl, longFixture, type FixtureShape } from "../../frontend/src/longfixture.ts";

const [shape, sid, out] = process.argv.slice(2);
writeFileSync(out, jsonl(longFixture(shape as FixtureShape, undefined, sid).events));
