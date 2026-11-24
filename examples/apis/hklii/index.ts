interface HTTPRequest {
  method?: string;
  url: string;
  headers?: Record<string, string>;
  query_params?: Record<string, string>;
  timeout_ms?: number;
}

interface HTTPResponse {
  ok: boolean;
  status: number;
  headers: Record<string, string>;
  body: string;
  truncated?: boolean;
}

interface ExtHost {
  readonly log: {
    debug(msg: string): void;
    info(msg: string): void;
    warn(msg: string): void;
    error(msg: string): void;
  };
  readonly http: {
    request(req: HTTPRequest): HTTPResponse;
  };
}

interface SearchArgs {
  query?: string;
  count?: number;
  disable_fuzzy?: boolean;
  base_url?: string;
}

interface ReadArgs {
  path?: string;
  lang?: string;
  abbr?: string;
  year?: string;
  num?: string;
  start?: number;
  max_chars?: number;
  base_url?: string;
}

const DEFAULT_BASE_URL = "https://www.hklii.hk";
const MAX_COUNT = 2500;

const NAMED_ENTITIES: Record<string, string> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: "\"",
  apos: "'",
  nbsp: " ",
  mdash: "—",
  ndash: "–",
  rsquo: "’",
  lsquo: "‘",
  rdquo: "”",
  ldquo: "“",
  hellip: "…",
  sect: "§",
  para: "¶",
  copy: "©",
  reg: "®",
};

function resolveBaseURL(raw?: string): string {
  const s = (raw || "").trim();
  const base = s || DEFAULT_BASE_URL;
  return base.replace(/\/+$/, "");
}

function decodeHTMLEntities(input: string): string {
  return input.replace(/&(#x[0-9a-fA-F]+|#\d+|[a-zA-Z]+);/g, (full, ent: string) => {
    if (ent.startsWith("#x") || ent.startsWith("#X")) {
      const cp = parseInt(ent.slice(2), 16);
      return Number.isFinite(cp) && cp > 0 && cp <= 0x10ffff ? String.fromCodePoint(cp) : full;
    }
    if (ent.startsWith("#")) {
      const cp = parseInt(ent.slice(1), 10);
      return Number.isFinite(cp) && cp > 0 && cp <= 0x10ffff ? String.fromCodePoint(cp) : full;
    }
    const named = NAMED_ENTITIES[ent.toLowerCase()];
    return named !== undefined ? named : full;
  });
}

function judgmentText(html: string): string {
  if (!html) return "";
  let s = html.replace(/<\/(p|tr|table|div|blockquote|li|h[1-6])>\s*|<\s*(br|p|tr|li|blockquote)\b[^>]*>/gi, "\n");
  s = s.replace(/<[^>]*>/g, " ");
  s = decodeHTMLEntities(s);
  const lines = s.split("\n").map((ln) => ln.replace(/[^\S\n]+/g, " ").trim());
  return lines.join("\n").replace(/\n{3,}/g, "\n\n").trim();
}

function resolveIdentity(args: ReadArgs): { lang: string; abbr: string; year: string; num: string } {
  let lang = (args.lang || "").trim();
  let abbr = (args.abbr || "").trim();
  let year = (args.year || "").trim();
  let num = (args.num || "").trim();

  const rawPath = (args.path || "").trim();
  if (rawPath !== "") {
    const parts = rawPath.replace(/^\/+|\/+$/g, "").split("/");
    if (parts.length !== 5 || parts[1] !== "cases") {
      throw new Error(`legal hklii: path "${rawPath}" is not a case judgment path (want /{lang}/cases/{abbr}/{year}/{num})`);
    }
    if (!lang) lang = parts[0];
    abbr = parts[2];
    year = parts[3];
    num = parts[4];
  }

  if (!lang) lang = "en";
  if (!abbr || !year || !num) {
    throw new Error("legal hklii: need a case path or abbr+year+num to fetch a judgment");
  }
  return { lang, abbr, year, num };
}

export function search(args: SearchArgs, host: ExtHost): Record<string, unknown> {
  const query = (args.query || "").trim();
  if (!query) {
    throw new Error("legal hklii: query is required");
  }
  let count = typeof args.count === "number" && args.count > 0 ? Math.floor(args.count) : 10;
  if (count > MAX_COUNT) count = MAX_COUNT;

  const base = resolveBaseURL(args.base_url);
  const started = Date.now();
  const resp = host.http.request({
    method: "GET",
    url: `${base}/api/simplesearch`,
    query_params: {
      searchstring: query,
      disablefuzzy: args.disable_fuzzy ? "1" : "0",
    },
    headers: {
      Accept: "application/json",
      "User-Agent": "agentflow",
    },
  });
  const timeCostMs = Date.now() - started;

  if (resp.status !== 200) {
    throw new Error(`legal hklii: status ${resp.status}: ${(resp.body || "").slice(0, 512)}`);
  }

  let parsed: {
    count?: number;
    results?: Array<{
      title?: string;
      path?: string;
      pub_date?: string;
      db?: string;
      act?: string;
      neutral?: string;
      parallel?: string;
      coram?: string;
      parties?: string;
    }>;
  };
  try {
    parsed = JSON.parse(resp.body);
  } catch (err) {
    throw new Error(`legal hklii: decode search response: ${String(err)}`);
  }

  const rawResults = Array.isArray(parsed.results) ? parsed.results.slice(0, count) : [];
  const results = rawResults.map((it) => {
    const item: Record<string, unknown> = {
      title: it.title || "",
      path: it.path || "",
      url: base + (it.path || ""),
      court: it.db || "",
      date: it.pub_date || "",
    };
    if (it.neutral) item.neutral = it.neutral;
    if (it.parallel) item.parallel = it.parallel;
    if (it.act) item.action = it.act;
    if (it.parties) item.parties = it.parties;
    if (it.coram) item.coram = it.coram;
    return item;
  });

  return {
    ok: true,
    engine: "hklii",
    query,
    total: typeof parsed.count === "number" ? parsed.count : results.length,
    count: results.length,
    results,
    time_cost_ms: timeCostMs,
  };
}

export function read(args: ReadArgs, host: ExtHost): Record<string, unknown> {
  const id = resolveIdentity(args);
  const base = resolveBaseURL(args.base_url);

  const resp = host.http.request({
    method: "GET",
    url: `${base}/api/getjudgment`,
    query_params: {
      lang: id.lang,
      abbr: id.abbr,
      year: id.year,
      num: id.num,
    },
    headers: {
      Accept: "application/json",
      "User-Agent": "agentflow",
    },
  });

  if (resp.status === 404) {
    throw new Error("legal hklii: not found (no such judgment, or that language version does not exist)");
  }
  if (resp.status !== 200) {
    throw new Error(`legal hklii: status ${resp.status}: ${(resp.body || "").slice(0, 512)}`);
  }

  let jr: {
    date?: string;
    db?: string;
    neutral?: string;
    content?: string;
    doc?: string;
    cases?: Array<{ title?: string; act?: string }>;
    parallel_citation?: string[];
    has_translation?: boolean;
  };
  try {
    jr = JSON.parse(resp.body);
  } catch (err) {
    throw new Error(`legal hklii: decode judgment response: ${String(err)}`);
  }

  let text = judgmentText(jr.content || "");
  const fullLen = text.length;
  const start = typeof args.start === "number" && args.start > 0 ? Math.floor(args.start) : 0;
  let truncated = false;
  if (start > 0) {
    text = start < text.length ? text.slice(start) : "";
    truncated = true;
  }
  const maxChars = typeof args.max_chars === "number" && args.max_chars > 0 ? Math.floor(args.max_chars) : 0;
  if (maxChars > 0 && text.length > maxChars) {
    text = text.slice(0, maxChars);
    truncated = true;
  }

  const title = (Array.isArray(jr.cases) && jr.cases.length > 0 && jr.cases[0].title) || jr.neutral || "";
  const out: Record<string, unknown> = {
    ok: true,
    engine: "hklii",
    title,
    neutral: jr.neutral || "",
    court: jr.db || "",
    date: jr.date || "",
    text,
    text_len: fullLen,
  };
  if (Array.isArray(jr.parallel_citation) && jr.parallel_citation.length > 0) {
    out.parallel_citation = jr.parallel_citation;
  }
  if (jr.doc) out.doc = jr.doc;
  if (truncated) out.truncated = true;
  if (jr.has_translation) out.has_translation = true;
  return out;
}
