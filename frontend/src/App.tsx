import React, { useCallback, useEffect, useReducer, useRef, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import {
  BookOpen,
  CheckCircle2,
  ChevronRight,
  Download,
  FileSpreadsheet,
  FilePlus2,
  FolderOpen,
  HelpCircle,
  Keyboard,
  Loader2,
  Lock,
  Plus,
  RefreshCw,
  Settings,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react';
import { api, getAuthToken, setAuthToken } from './api';
import type {
  AppSettings,
  ProblemSet,
  RubricPartScore,
  Submission,
} from './types';
import { GradeView } from './GradeView';

/* ─── tiny state machine ─────────────────────────────────────────── */
type Page = 'login' | 'library' | 'grade';
interface State {
  page: Page;
  settings: AppSettings | null;
  psets: ProblemSet[];
  activePset: ProblemSet | null;
  subs: Submission[];
  activeSubId: number | null;
  draftParts: RubricPartScore[];
  draftNotes: string;
  activePartId: string | null;
  toast: string | null;
  busy: boolean;
}
type Action =
  | { type: 'SET_PAGE'; page: Page }
  | { type: 'SET_SETTINGS'; settings: AppSettings }
  | { type: 'SET_PSETS'; psets: ProblemSet[] }
  | { type: 'OPEN_PSET'; pset: ProblemSet; subs: Submission[] }
  | { type: 'SET_SUBS'; subs: Submission[] }
  | { type: 'SELECT_SUB'; sub: Submission }
  | { type: 'SET_DRAFT_PARTS'; parts: RubricPartScore[] }
  | { type: 'SET_DRAFT_NOTES'; notes: string }
  | { type: 'SET_ACTIVE_PART'; partId: string | null }
  | { type: 'SET_PART_SCORE'; partId: string; score: number }
  | { type: 'RESET_PARTS_TO_AI' }
  | { type: 'UPDATE_SUB'; sub: Submission }
  | { type: 'TOAST'; msg: string | null }
  | { type: 'SET_BUSY'; v: boolean };

const init: State = {
  page: 'login',
  settings: null,
  psets: [],
  activePset: null,
  subs: [],
  activeSubId: null,
  draftParts: [],
  draftNotes: '',
  activePartId: null,
  toast: null,
  busy: false,
};

function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'SET_PAGE': return { ...s, page: a.page };
    case 'SET_SETTINGS': return { ...s, settings: a.settings };
    case 'SET_PSETS': return { ...s, psets: a.psets };
    case 'OPEN_PSET': return { ...s, page: 'grade', activePset: a.pset, subs: a.subs, activeSubId: a.subs[0]?.id ?? null };
    case 'SET_SUBS': return { ...s, subs: a.subs };
    case 'SELECT_SUB': {
      const parts = a.sub.result?.rubric_parts?.map(p => ({ ...p })) ?? [];
      return { ...s, activeSubId: a.sub.id, draftParts: parts, draftNotes: a.sub.teacher_notes || '', activePartId: parts[0]?.part_id ?? null };
    }
    case 'SET_DRAFT_PARTS': return { ...s, draftParts: a.parts };
    case 'SET_DRAFT_NOTES': return { ...s, draftNotes: a.notes };
    case 'SET_ACTIVE_PART': return { ...s, activePartId: a.partId };
    case 'SET_PART_SCORE':
      return { ...s, draftParts: s.draftParts.map(p => p.part_id === a.partId ? { ...p, teacher_score: Math.max(0, Math.min(p.max_score, a.score)) } : p) };
    case 'RESET_PARTS_TO_AI':
      return { ...s, draftParts: s.draftParts.map(p => ({ ...p, teacher_score: null })) };
    case 'UPDATE_SUB': {
      const subs = s.subs.map(x => x.id === a.sub.id ? a.sub : x);
      const parts = a.sub.result?.rubric_parts?.map(p => ({ ...p })) ?? s.draftParts;
      return { ...s, subs, draftParts: s.activeSubId === a.sub.id ? parts : s.draftParts };
    }
    case 'TOAST': return { ...s, toast: a.msg };
    case 'SET_BUSY': return { ...s, busy: a.v };
  }
}

/* ─── App shell ──────────────────────────────────────────────────── */
export const App: React.FC = () => {
  const [st, dispatch] = useReducer(reducer, init);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const toast = useCallback((msg: string) => {
    dispatch({ type: 'TOAST', msg });
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => dispatch({ type: 'TOAST', msg: null }), 2600);
  }, []);

  /* Boot: check auth then load library */
  useEffect(() => {
    (async () => {
      try {
        const status = await api.authStatus();
        if (status.auth_required && !status.authenticated) {
          dispatch({ type: 'SET_PAGE', page: 'login' });
          return;
        }
        const [cfg, psets] = await Promise.all([api.getSettings(), api.listProblemSets()]);
        dispatch({ type: 'SET_SETTINGS', settings: cfg });
        dispatch({ type: 'SET_PSETS', psets });
        dispatch({ type: 'SET_PAGE', page: 'library' });
      } catch {
        dispatch({ type: 'SET_PAGE', page: 'login' });
      }
    })();
  }, []);

  /* Auto-poll while any sub is grading */
  useEffect(() => {
    if (!st.activePset || !st.subs.some(s => s.status === 'grading')) return;
    const t = setInterval(async () => {
      const subs = await api.listSubmissions(st.activePset!.id);
      dispatch({ type: 'SET_SUBS', subs });
      const psets = await api.listProblemSets();
      dispatch({ type: 'SET_PSETS', psets });
    }, 2000);
    return () => clearInterval(t);
  }, [st.activePset, st.subs]);

  const openPset = async (ps: ProblemSet) => {
    const subs = await api.listSubmissions(ps.id);
    dispatch({ type: 'OPEN_PSET', pset: ps, subs });
  };

  const activeSub = st.subs.find(s => s.id === st.activeSubId) ?? null;

  /* Pages */
  if (st.page === 'login') return <LoginPage onDone={async () => {
    const [cfg, psets] = await Promise.all([api.getSettings(), api.listProblemSets()]);
    dispatch({ type: 'SET_SETTINGS', settings: cfg });
    dispatch({ type: 'SET_PSETS', psets });
    dispatch({ type: 'SET_PAGE', page: 'library' });
  }} />;

  if (st.page === 'library') return (
    <LibraryPage
      psets={st.psets}
      settings={st.settings}
      onOpen={openPset}
      onPsetsChange={async () => {
        const psets = await api.listProblemSets();
        dispatch({ type: 'SET_PSETS', psets });
      }}
      onSettingsChange={s => dispatch({ type: 'SET_SETTINGS', settings: s })}
    />
  );

  return (
    <GradeView
      pset={st.activePset!}
      subs={st.subs}
      activeSub={activeSub}
      draftParts={st.draftParts}
      draftNotes={st.draftNotes}
      activePartId={st.activePartId}
      onBack={() => dispatch({ type: 'SET_PAGE', page: 'library' })}
      onSelectSub={sub => dispatch({ type: 'SELECT_SUB', sub })}
      onSetPartScore={(partId, score) => dispatch({ type: 'SET_PART_SCORE', partId, score })}
      onSetActivePartId={partId => dispatch({ type: 'SET_ACTIVE_PART', partId })}
      onSetNotes={notes => dispatch({ type: 'SET_DRAFT_NOTES', notes })}
      onResetToAI={() => dispatch({ type: 'RESET_PARTS_TO_AI' })}
      onSubUpdated={sub => dispatch({ type: 'UPDATE_SUB', sub })}
      onSubsRefresh={async () => {
        const subs = await api.listSubmissions(st.activePset!.id);
        dispatch({ type: 'SET_SUBS', subs });
        const psets = await api.listProblemSets();
        dispatch({ type: 'SET_PSETS', psets });
        const updated = psets.find(p => p.id === st.activePset!.id);
        if (updated) dispatch({ type: 'OPEN_PSET', pset: updated, subs });
      }}
      toast={toast}
    />
  );
};

/* ─── Login page ─────────────────────────────────────────────────── */
const LoginPage: React.FC<{ onDone: () => void }> = ({ onDone }) => {
  const [passcode, setPasscode] = useState('');
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setErr('');
    try {
      const res = await api.login(passcode);
      setAuthToken(res.token);
      await onDone();
    } catch (error: unknown) {
      setErr(error instanceof Error ? error.message : '口令不正确');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#F6F5F1]">
      <form onSubmit={submit} className="w-80 bg-white rounded-2xl border border-[#E5E2DA] p-8 space-y-5 shadow-sm">
        <div className="w-8 h-8 rounded-xl bg-[#191918] flex items-center justify-center">
          <Lock className="w-4 h-4 text-white" />
        </div>
        <div>
          <div className="text-sm font-semibold text-[#191918]">PsychGrader</div>
          <div className="text-xs text-[#78756E] mt-0.5">AP Psychology 阅卷台</div>
        </div>
        {err && <div className="text-xs text-[#9F1239] bg-[#FFE4E6] rounded-lg px-3 py-2">{err}</div>}
        <input
          type="password"
          autoFocus
          value={passcode}
          onChange={e => setPasscode(e.target.value)}
          placeholder="访问口令"
          className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3.5 py-2.5 text-sm focus:outline-none focus:border-[#191918]"
        />
        <button
          type="submit"
          disabled={loading || !passcode}
          className="w-full py-2.5 rounded-xl bg-[#191918] text-white text-xs font-medium disabled:opacity-40"
        >
          {loading ? '验证中…' : '进入'}
        </button>
      </form>
    </div>
  );
};

/* ─── Library page ───────────────────────────────────────────────── */
const LibraryPage: React.FC<{
  psets: ProblemSet[];
  settings: AppSettings | null;
  onOpen: (ps: ProblemSet) => void;
  onPsetsChange: () => void;
  onSettingsChange: (s: AppSettings) => void;
}> = ({ psets, settings, onOpen, onPsetsChange, onSettingsChange }) => {
  const [showCreate, setShowCreate] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [editTarget, setEditTarget] = useState<ProblemSet | null>(null);

  return (
    <div className="min-h-screen bg-[#F6F5F1] flex flex-col">
      {/* Fixed-height top bar — no hover height changes */}
      <header className="h-14 shrink-0 bg-white border-b border-[#E5E2DA] flex items-center justify-between px-6">
        <div className="flex items-center gap-2.5">
          <span className="w-2 h-2 rounded-full bg-[#1F4E3B]" />
          <span className="text-sm font-semibold tracking-tight text-[#191918]">PsychGrader</span>
          <span className="text-[11px] px-1.5 py-0.5 rounded bg-[#F2F0EA] text-[#6E6B65]">题库</span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowSettings(true)}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-xs font-medium transition ${
              settings?.has_api_key
                ? 'border-[#E2DFD7] bg-white text-[#3F3D39] hover:bg-[#F5F3EE]'
                : 'border-[#FDE68A] bg-[#FFFBEB] text-[#92400E] hover:bg-[#FEF3C7]'
            }`}
          >
            <Settings className="w-3.5 h-3.5" />
            {settings?.has_api_key ? settings.model : '需要配置 API Key'}
          </button>
          <button
            onClick={() => { setEditTarget(null); setShowCreate(true); }}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#191918] hover:bg-[#2E2D2A] text-white text-xs font-medium"
          >
            <Plus className="w-3.5 h-3.5" /> 新建题集
          </button>
        </div>
      </header>

      {/* Grid of problem sets */}
      <main className="flex-1 p-8 max-w-5xl mx-auto w-full">
        {psets.length === 0 ? (
          <div className="mt-24 flex flex-col items-center gap-4 text-center">
            <div className="w-14 h-14 rounded-2xl bg-[#F2F0EA] flex items-center justify-center">
              <BookOpen className="w-6 h-6 text-[#8C8880]" />
            </div>
            <div className="text-sm font-medium text-[#3F3D39]">还没有题集</div>
            <div className="text-xs text-[#8C8880]">点击右上角「新建题集」录入 EBQ / AAQ 题目和 Source 材料</div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {psets.map(ps => (
              <PsetCard
                key={ps.id}
                ps={ps}
                onOpen={() => onOpen(ps)}
                onEdit={() => { setEditTarget(ps); setShowCreate(true); }}
                onDelete={async () => {
                  if (!confirm(`确定删除题集「${ps.title}」及其所有答卷吗？`)) return;
                  await api.deleteProblemSet(ps.id);
                  onPsetsChange();
                }}
              />
            ))}
          </div>
        )}
      </main>

      {showCreate && (
        <PsetEditModal
          initial={editTarget}
          onClose={() => setShowCreate(false)}
          onSaved={() => { setShowCreate(false); onPsetsChange(); }}
        />
      )}

      {showSettings && (
        <SettingsModal
          settings={settings}
          onClose={() => setShowSettings(false)}
          onUpdated={s => { onSettingsChange(s); setShowSettings(false); }}
        />
      )}
    </div>
  );
};

/* ─── Problem set card ───────────────────────────────────────────── */
const PsetCard: React.FC<{
  ps: ProblemSet;
  onOpen: () => void;
  onEdit: () => void;
  onDelete: () => void;
}> = ({ ps, onOpen, onEdit, onDelete }) => {
  const pct = ps.total_count ? Math.round((ps.reviewed_count / ps.total_count) * 100) : 0;
  return (
    <div
      onClick={onOpen}
      className="bg-white border border-[#E5E2DA] rounded-2xl p-5 cursor-pointer hover:border-[#C8C4BA] hover:shadow-sm transition-all group flex flex-col gap-3"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-1.5 mb-1">
            <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded ${
              ps.question_type === 'EBQ'
                ? 'bg-[#FEF3C7] text-[#92400E]'
                : 'bg-[#E0F2FE] text-[#075985]'
            }`}>
              {ps.question_type}
            </span>
          </div>
          <div className="text-sm font-medium text-[#191918] leading-snug line-clamp-2">{ps.title}</div>
        </div>
        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition shrink-0">
          <button
            onClick={e => { e.stopPropagation(); onEdit(); }}
            className="p-1.5 rounded-lg hover:bg-[#F2F0EA] text-[#6E6B65]"
          >
            <Settings className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={e => { e.stopPropagation(); onDelete(); }}
            className="p-1.5 rounded-lg hover:bg-[#FFE4E6] text-[#9F1239]"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {ps.total_count > 0 ? (
        <div className="space-y-1.5">
          <div className="flex justify-between text-[11px] text-[#6E6B65]">
            <span>{ps.reviewed_count} / {ps.total_count} 已复核</span>
            <span className="tabular-nums">{ps.avg_score ? `均分 ${ps.avg_score.toFixed(1)}/7` : '—'}</span>
          </div>
          <div className="h-1.5 bg-[#F2F0EA] rounded-full overflow-hidden">
            <div
              className="h-full bg-[#1F4E3B] rounded-full transition-all"
              style={{ width: `${pct}%` }}
            />
          </div>
        </div>
      ) : (
        <div className="text-[11px] text-[#A39F96]">尚未导入答卷</div>
      )}

      <div className="flex items-center justify-between pt-0.5">
        <span className="text-[11px] text-[#A39F96]">
          {ps.sources?.length ?? 0} 篇文献 · {(ps.ineligible_concepts ?? []).length} 个禁用词
        </span>
        <ChevronRight className="w-4 h-4 text-[#B8B4AA] group-hover:text-[#191918] transition" />
      </div>
    </div>
  );
};

/* ─── Problem set create / edit modal ───────────────────────────── */
const PsetEditModal: React.FC<{
  initial: ProblemSet | null;
  onClose: () => void;
  onSaved: () => void;
}> = ({ initial, onClose, onSaved }) => {
  const [title, setTitle] = useState(initial?.title ?? '');
  const [qType, setQType] = useState<'EBQ' | 'AAQ'>(initial?.question_type ?? 'EBQ');
  const [promptText, setPromptText] = useState(initial?.prompt_text ?? '');
  const [sources, setSources] = useState(
    initial?.sources?.length
      ? initial.sources
      : [
          { id: 'Source 1', label: 'Source 1', title: '', summary: '' },
          { id: 'Source 2', label: 'Source 2', title: '', summary: '' },
          { id: 'Source 3', label: 'Source 3', title: '', summary: '' },
        ]
  );
  const [ineligibleRaw, setIneligibleRaw] = useState(
    (initial?.ineligible_concepts ?? ['distractor', 'mean', 'random assignment', 'rehearsal', 'retrieval', 'standard deviation', 'statistical significance']).join(', ')
  );
  const [scoringNotes, setScoringNotes] = useState(initial?.scoring_notes ?? '');
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');

  const save = async () => {
    if (!title.trim()) { setErr('请填写题集名称'); return; }
    setSaving(true);
    setErr('');
    try {
      const payload = {
        title: title.trim(),
        question_type: qType,
        prompt_text: promptText.trim(),
        sources: qType === 'EBQ' ? sources : sources.slice(0, 1),
        ineligible_concepts: ineligibleRaw.split(/[,，\n]/).map(s => s.trim()).filter(Boolean),
        scoring_notes: scoringNotes.trim(),
      };
      if (initial) await api.updateProblemSet(initial.id, payload);
      else await api.createProblemSet(payload);
      onSaved();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/30 backdrop-blur-[2px] flex items-start justify-center pt-12 p-4 overflow-y-auto">
      <div className="w-full max-w-2xl bg-white rounded-2xl border border-[#E2DFD7] shadow-xl mb-12">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#EAE7DF]">
          <div className="text-sm font-semibold text-[#191918]">{initial ? '编辑题集' : '新建题集'}</div>
          <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-[#F2F0EA] text-[#78756E]"><X className="w-4 h-4" /></button>
        </div>
        <div className="p-6 space-y-4 text-xs">
          {err && <div className="text-[#9F1239] bg-[#FFE4E6] rounded-xl px-3 py-2">{err}</div>}
          <div className="grid grid-cols-4 gap-3">
            <div className="col-span-3">
              <label className="block font-medium text-[#3F3D39] mb-1">题集名称</label>
              <input value={title} onChange={e => setTitle(e.target.value)} placeholder="例：Unit 2 Quiz — Spaced Practice EBQ" className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3 py-2 focus:outline-none focus:border-[#191918]" />
            </div>
            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">题型</label>
              <select value={qType} onChange={e => setQType(e.target.value as 'EBQ' | 'AAQ')} className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2">
                <option value="EBQ">EBQ · 7 分</option>
                <option value="AAQ">AAQ · 7 分</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">题干 (Prompt Stem)</label>
            <textarea rows={2} value={promptText} onChange={e => setPromptText(e.target.value)} className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] p-3 leading-relaxed focus:outline-none focus:border-[#191918]" />
          </div>

          {qType === 'EBQ' && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <label className="font-medium text-[#3F3D39]">Source 文献摘要（供 AI 核查引用准确性）</label>
                <button type="button" onClick={() => setSources(prev => [...prev, { id: `Source ${prev.length + 1}`, label: `Source ${prev.length + 1}`, title: '', summary: '' }])} className="text-[11px] text-[#57544E] hover:text-[#191918] inline-flex items-center gap-0.5">
                  <Plus className="w-3 h-3" /> 添加
                </button>
              </div>
              {sources.map((src, i) => (
                <div key={i} className="rounded-xl bg-[#FAF9F6] border border-[#E5E2DA] p-3 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-[#191918]">{src.label}</span>
                    <input value={src.title} onChange={e => { const v = e.target.value; setSources(prev => prev.map((s, j) => j === i ? { ...s, title: v } : s)); }} placeholder="文献标题 / 作者年份" className="flex-1 rounded-lg border border-[#E2DFD7] px-2.5 py-1 bg-white" />
                    {sources.length > 1 && <button onClick={() => setSources(prev => prev.filter((_, j) => j !== i))} className="text-[#9F1239] p-1 rounded hover:bg-[#FFE4E6]"><Trash2 className="w-3 h-3" /></button>}
                  </div>
                  <textarea rows={2} value={src.summary} onChange={e => { const v = e.target.value; setSources(prev => prev.map((s, j) => j === i ? { ...s, summary: v } : s)); }} placeholder="粘贴该 Source 的实验设计与核心结果，AI 将据此核查学生引用是否张冠李戴…" className="w-full rounded-lg border border-[#EFECE4] bg-white p-2.5 leading-relaxed" />
                </div>
              ))}
            </div>
          )}

          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">禁用词 (Ineligible Concepts，逗号分隔)</label>
            <input value={ineligibleRaw} onChange={e => setIneligibleRaw(e.target.value)} className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3 py-2 font-mono" />
          </div>

          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">补充采分说明（可选，注入给模型）</label>
            <textarea rows={2} value={scoringNotes} onChange={e => setScoringNotes(e.target.value)} className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] p-3 leading-relaxed" />
          </div>
        </div>
        <div className="flex justify-end gap-2 px-6 py-4 border-t border-[#EAE7DF]">
          <button onClick={onClose} className="px-4 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE]">取消</button>
          <button onClick={save} disabled={saving} className="px-5 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium disabled:opacity-50">
            {saving ? '保存中…' : '保存'}
          </button>
        </div>
      </div>
    </div>
  );
};

/* ─── Settings modal ─────────────────────────────────────────────── */
const SettingsModal: React.FC<{
  settings: AppSettings | null;
  onClose: () => void;
  onUpdated: (s: AppSettings) => void;
}> = ({ settings, onClose, onUpdated }) => {
  const [baseUrl, setBaseUrl] = useState(settings?.api_base_url ?? 'https://api.deepseek.com');
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState(settings?.model ?? 'deepseek-flash');
  const [thinking, setThinking] = useState(settings?.thinking_enabled ?? true);
  const [effort, setEffort] = useState(settings?.reasoning_effort ?? 'medium');
  const [concurrency, setConcurrency] = useState(settings?.concurrency ?? 4);
  const [testing, setTesting] = useState(false);
  const [testMsg, setTestMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const test = async () => {
    setTesting(true); setTestMsg(null);
    try {
      const r = await api.testLLM({ api_base_url: baseUrl, api_key: apiKey || undefined, model });
      setTestMsg({ ok: true, text: `连通 (${r.model})：${r.reply}` });
    } catch (e: unknown) {
      setTestMsg({ ok: false, text: e instanceof Error ? e.message : '失败' });
    } finally { setTesting(false); }
  };

  const save = async () => {
    setSaving(true);
    try {
      const p: Record<string, unknown> = { api_base_url: baseUrl.trim(), model, thinking_enabled: thinking, reasoning_effort: effort, concurrency };
      if (apiKey.trim()) p.api_key = apiKey.trim();
      const next = await api.updateSettings(p);
      onUpdated(next);
    } finally { setSaving(false); }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/30 backdrop-blur-[2px] flex items-center justify-center p-4">
      <div className="w-full max-w-md bg-white rounded-2xl border border-[#E2DFD7] shadow-xl">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#EAE7DF]">
          <div className="text-sm font-semibold text-[#191918]">DeepSeek 配置</div>
          <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-[#F2F0EA] text-[#78756E]"><X className="w-4 h-4" /></button>
        </div>
        <div className="p-6 space-y-4 text-xs">
          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">API Base URL</label>
            <input value={baseUrl} onChange={e => setBaseUrl(e.target.value)} className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3 py-2 font-mono" />
          </div>
          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">
              API Key {settings?.has_api_key && <span className="text-[#15803D]">（已配置 {settings.api_key_masked}）</span>}
            </label>
            <input type="password" value={apiKey} onChange={e => setApiKey(e.target.value)} placeholder={settings?.has_api_key ? '留空保持不变' : 'sk-…'} className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3 py-2 font-mono" />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">模型</label>
              <select value={model} onChange={e => setModel(e.target.value)} className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2">
                <option value="deepseek-flash">deepseek-flash (V4.1 Flash)</option>
                <option value="deepseek-v4-pro">deepseek-v4-pro (V4 Pro)</option>
              </select>
            </div>
            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">并发数</label>
              <input type="number" min={1} max={12} value={concurrency} onChange={e => setConcurrency(Number(e.target.value) || 4)} className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2" />
            </div>
          </div>
          <label className="flex items-center justify-between p-3 rounded-xl bg-[#FAF9F6] border border-[#E5E2DA] cursor-pointer">
            <div>
              <div className="font-medium text-[#191918]">Thinking 推理链</div>
              <div className="text-[11px] text-[#78756E] mt-0.5">先逐步验算再出 JSON，准确率更高</div>
            </div>
            <div className="flex items-center gap-2">
              {thinking && (
                <select value={effort} onChange={e => setEffort(e.target.value)} className="rounded-lg border border-[#DCD9D0] bg-white px-2 py-1 text-[11px]" onClick={e => e.stopPropagation()}>
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                </select>
              )}
              <input type="checkbox" checked={thinking} onChange={e => setThinking(e.target.checked)} className="w-4 h-4 accent-[#191918]" />
            </div>
          </label>
          {testMsg && (
            <div className={`p-2.5 rounded-xl text-xs ${testMsg.ok ? 'bg-[#DCFCE7] text-[#166534]' : 'bg-[#FFE4E6] text-[#9F1239]'}`}>
              {testMsg.text}
            </div>
          )}
        </div>
        <div className="flex items-center justify-between px-6 py-4 border-t border-[#EAE7DF]">
          <button onClick={test} disabled={testing} className="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#3F3D39] hover:bg-[#F5F3EE]">
            {testing && <Loader2 className="w-3.5 h-3.5 animate-spin" />} 测试连通
          </button>
          <div className="flex gap-2">
            <button onClick={onClose} className="px-4 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE]">关闭</button>
            <button onClick={save} disabled={saving} className="px-5 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium disabled:opacity-50">保存</button>
          </div>
        </div>
      </div>
    </div>
  );
};
