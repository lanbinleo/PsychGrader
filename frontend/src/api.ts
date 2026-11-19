import type {
  AppSettings,
  CEDEntry,
  ProblemSet,
  RubricPartScore,
  SplitSegment,
  Submission,
} from './types';

const TOKEN_KEY = 'psychgrader_token';

export function getAuthToken(): string {
  return localStorage.getItem(TOKEN_KEY) || '';
}

export function setAuthToken(tok: string) {
  if (tok) localStorage.setItem(TOKEN_KEY, tok);
  else localStorage.removeItem(TOKEN_KEY);
}

async function req<T>(url: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers || {});
  const tok = getAuthToken();
  if (tok) headers.set('Authorization', `Bearer ${tok}`);
  if (!(init?.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const resp = await fetch(url, { ...init, headers });
  if (resp.status === 401) {
    const data = await resp.json().catch(() => ({}));
    throw Object.assign(new Error(data.error || 'Unauthorized'), { status: 401 });
  }
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    throw new Error(data.error || `HTTP ${resp.status}`);
  }
  return resp.json();
}

export const api = {
  authStatus: () => req<{ auth_required: boolean; authenticated: boolean }>('/api/auth/status'),
  login: (passcode: string) =>
    req<{ token: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ passcode }),
    }),

  getSettings: () => req<AppSettings>('/api/settings'),
  updateSettings: (payload: Record<string, unknown>) =>
    req<AppSettings>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  testLLM: (payload: { api_base_url?: string; api_key?: string; model?: string }) =>
    req<{ ok: boolean; reply: string; model: string }>('/api/settings/test', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),

  searchCED: (q: string) => req<CEDEntry[]>(`/api/ced/search?q=${encodeURIComponent(q)}`),

  listProblemSets: () => req<ProblemSet[]>('/api/problem-sets'),
  createProblemSet: (ps: Partial<ProblemSet>) =>
    req<ProblemSet>('/api/problem-sets', {
      method: 'POST',
      body: JSON.stringify(ps),
    }),
  updateProblemSet: (id: number, ps: Partial<ProblemSet>) =>
    req<ProblemSet>(`/api/problem-sets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(ps),
    }),
  deleteProblemSet: (id: number) =>
    req<{ deleted: number }>(`/api/problem-sets/${id}`, { method: 'DELETE' }),

  parsePDF: (psetId: number, file: File, mode: string, pagesPerStudent: number) => {
    const fd = new FormData();
    fd.append('file', file);
    fd.append('mode', mode);
    fd.append('pages_per_student', String(pagesPerStudent));
    return req<{
      filename: string;
      page_count: number;
      pages: string[];
      segments: SplitSegment[];
    }>(`/api/problem-sets/${psetId}/parse-pdf`, { method: 'POST', body: fd });
  },

  importSubmissions: (psetId: number, segments: SplitSegment[], autoGrade: boolean) =>
    req<{ imported_count: number; submissions: Submission[] }>(
      `/api/problem-sets/${psetId}/import`,
      {
        method: 'POST',
        body: JSON.stringify({ segments, auto_grade: autoGrade }),
      }
    ),

  listSubmissions: (psetId: number) =>
    req<Submission[]>(`/api/problem-sets/${psetId}/submissions`),
  getSubmission: (id: number) => req<Submission>(`/api/submissions/${id}`),
  gradeOne: (id: number) =>
    req<Submission>(`/api/submissions/${id}/grade`, { method: 'POST' }),
  gradeAll: (psetId: number, onlyUnscored: boolean) =>
    req<{ queued: number }>(`/api/problem-sets/${psetId}/grade-all`, {
      method: 'POST',
      body: JSON.stringify({ only_unscored: onlyUnscored }),
    }),
  reviewSubmission: (
    id: number,
    finalScore: number,
    teacherNotes: string,
    rubricParts: RubricPartScore[]
  ) =>
    req<Submission>(`/api/submissions/${id}/review`, {
      method: 'PUT',
      body: JSON.stringify({
        final_score: finalScore,
        teacher_notes: teacherNotes,
        rubric_parts: rubricParts,
      }),
    }),
  deleteSubmission: (id: number) =>
    req<{ deleted: number }>(`/api/submissions/${id}`, { method: 'DELETE' }),

  exportCSVUrl: (psetId: number) => {
    const tok = getAuthToken();
    return `/api/problem-sets/${psetId}/export.csv${tok ? `?token=${encodeURIComponent(tok)}` : ''}`;
  },
};
