// A guardrail's second opinion (LLM judge) as compliance events record it
// (scripting/guardrail_judge_run.go): one guardrail.judge event per judged
// segment, the opinion under metadata.judge: verdict (benign, suspicious,
// attack, or none with an error), reason, agreed, coverage (full, or partial
// when the judge was not shown everything flagged), sha256 (the segment's, as
// on the detector event), latency_ms, cached, model.

export const JUDGE_EVENT_TYPE = "guardrail.judge";

const VERDICTS = { benign: "benign", suspicious: "suspicious", attack: "an attack" };

const ERRORS = {
  timeout: "timed out",
  call_failed: "the call failed",
  unparseable: "unreadable answer",
  judge_unavailable: "judge LLM unavailable",
  privacy: "judge LLM's privacy score too low",
  over_budget: "judge LLM over budget",
  dropped: "queue full",
  over_segment_cap: "too many flagged segments",
  category: "secret or personal data, never judged",
};

// judgeOf is the event's opinion, or null.
export const judgeOf = (metadata) => {
  const j = metadata?.judge;
  if (!j || typeof j !== "object" || typeof j.verdict !== "string") return null;
  return j;
};

export const hasOpinion = (j) => !!j && j.verdict !== "none" && !!VERDICTS[j.verdict];

// opinionLabel is one line saying what the judge thought.
export const opinionLabel = (j) => {
  if (!j) return "";
  if (!hasOpinion(j)) {
    const why = ERRORS[j.error] || (typeof j.error === "string" ? j.error : "");
    return why ? `Judge: no opinion (${why})` : "Judge: no opinion";
  }
  const reason = typeof j.reason === "string" && j.reason ? ` (${j.reason.replace(/_/g, " ")})` : "";
  return `Judge: ${VERDICTS[j.verdict]}${reason}`;
};

// opinionColor is the chip colour: benign (the judge disagrees with the
// block) stands out; agreement is quiet.
export const opinionColor = (j) => {
  if (!hasOpinion(j)) return "default";
  return j.verdict === "benign" ? "warning" : "success";
};

const key = (requestId, sha) => `${requestId || ""}|${sha || ""}`;

// judgeIndex maps request id + segment SHA-256 to the judge's opinion, so a
// detector event's segments can show it.
export const judgeIndex = (events) => {
  const index = new Map();
  (events || []).forEach((e) => {
    if (e?.event_type !== JUDGE_EVENT_TYPE) return;
    const j = judgeOf(e.metadata);
    if (!j || !j.sha256) return;
    index.set(key(e.metadata?.request_id, j.sha256), j);
  });
  return index;
};

export const lookupOpinion = (index, requestId, sha) => (index && sha ? index.get(key(requestId, sha)) || null : null);

// agreementByFilter summarises judge events per filter: of the flags the
// judge gave an opinion on, how many it found benign (it disagreed with the
// filter), how many of those it was shown in full, and how many got none.
export const agreementByFilter = (events) => {
  const byFilter = new Map();
  (events || []).forEach((e) => {
    if (e?.event_type !== JUDGE_EVENT_TYPE) return;
    const j = judgeOf(e.metadata);
    if (!j) return;
    const name = e.filter_name || "(unnamed filter)";
    const row = byFilter.get(name) || { filter: name, judged: 0, benign: 0, benignFull: 0, noOpinion: 0 };
    if (hasOpinion(j)) {
      row.judged += 1;
      if (j.verdict === "benign") {
        row.benign += 1;
        if (j.coverage === "full") row.benignFull += 1;
      }
    } else {
      row.noOpinion += 1;
    }
    byFilter.set(name, row);
  });
  return [...byFilter.values()].sort((a, b) => a.filter.localeCompare(b.filter));
};
