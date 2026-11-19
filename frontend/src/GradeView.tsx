import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { AnimatePresence, motion } from 'framer-motion';
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronRight,
  FilePlus2,
  FileSpreadsheet,
  HelpCircle,
  Info,
  Loader2,
  PanelLeft,
  RotateCcw,
  Sparkles,
  Upload,
  X,
} from 'lucide-react';
import { api } from './api';
import type {
  Annotation,
  AnnotationCategory,
  GradingTask,
  ProblemSet,
  RubricPartScore,
  SplitSegment,
  Submission,
} from './types';

/* ─── colour palette ─────────────────────────────────────────────── */
const CAT_STYLE: Record<AnnotationCategory, { bg: string; ring: string; dot: string; label: string }> = {
  claim:       { bg: 'bg-[#FEF3C7]', ring: 'ring-1 ring-[#D4A437]/40', dot: 'bg-[#D4A437]', label: 'Claim' },
  evidence:    { bg: 'bg-[#E0F2FE]', ring: 'ring-1 ring-[#5B9BC9]/40', dot: 'bg-[#5B9BC9]', label: 'Evidence' },
  explanation: { bg: 'bg-[#DCFCE7]', ring: 'ring-1 ring-[#6BA76F]/40', dot: 'bg-[#6BA76F]', label: 'Reasoning' },
  concept:     { bg: 'bg-[#FFEDD5]', ring: 'ring-1 ring-[#D88A4A]/40', dot: 'bg-[#D88A4A]', label: 'Concept' },
  issue:       { bg: 'bg-[#FFE4E6]', ring: 'ring-1 ring-[#C95B6E]/40', dot: 'bg-[#C95B6E]', label: 'Issue' },
};

const FLAG_LABELS: Record<string, { text: string; tone: 'err' | 'warn' }> = {
  missing_citation:     { text: '漏标 Source 出处',   tone: 'err'  },
  wrong_source:         { text: 'Source 编号标错',    tone: 'err'  },
  duplicate_source:     { text: '与 B 段同源冲突',   tone: 'err'  },
  duplicate_concept:    { text: '与 B 段概念重复',   tone: 'err'  },
  ineligible_concept:   { text: '误踩题干禁用词',    tone: 'err'  },
  definition_only:      { text: '纯背定义未结合材料', tone: 'warn' },
  direction_reversed:   { text: '实验结果方向说反',  tone: 'err'  },
  self_contradiction:   { text: '前后自相矛盾',      tone: 'err'  },
  oversimplified_claim: { text: '主张空泛',          tone: 'warn' },
};

/* ══════════════════════════════════════════════════════════════════
   GradeView
   Layout: [left queue drawer] | [paper — dominant] | [rubric rail]
   Queue: open by default, collapses to a floating pill on the left edge.
   Rubric: compact one-line rows; detail opens as a Popover above the row.
   ══════════════════════════════════════════════════════════════════ */
export interface GradeViewProps {
  pset: ProblemSet;
  subs: Submission[];
  activeSub: Submission | null;
  draftParts: RubricPartScore[];
  draftNotes: string;
  draftDirty: boolean;
  activePartId: string | null;
  batch: GradingTask | null;
  onBack: () => void;
  onSelectSub: (s: Submission) => void;
  onSetPartScore: (partId: string, score: number) => void;
  onSetActivePartId: (id: string | null) => void;
  onSetNotes: (n: string) => void;
  onResetToAI: () => void;
  onSubUpdated: (s: Submission, syncDraft?: boolean) => void;
  onSubsRefresh: () => Promise<Submission[]>;
  onBatchStarted: (task: GradingTask) => void;
  onCancelBatch: (id: number) => void;
  onResumeBatch: (id: number) => void;
  onDismissBatch: () => void;
  toast: (m: string) => void;
}

export const GradeView: React.FC<GradeViewProps> = ({
  pset, subs, activeSub, draftParts, draftNotes, draftDirty, activePartId, batch,
  onBack, onSelectSub, onSetPartScore, onSetActivePartId, onSetNotes, onResetToAI,
  onSubUpdated, onSubsRefresh, onBatchStarted, onCancelBatch, onResumeBatch, onDismissBatch, toast,
}) => {
  const [queueOpen, setQueueOpen]       = useState(true);   // open by default
  const [showImport, setShowImport]     = useState(false);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [showPrompt, setShowPrompt]     = useState(false);
  const [filter, setFilter]             = useState<'all' | 'unreviewed' | 'flagged'>('all');
  const [gradingIds, setGradingIds]     = useState<number[]>([]);
  const [saving, setSaving]             = useState(false);
  const [starting, setStarting]         = useState(false);
  const [exporting, setExporting]       = useState(false);

  const batchActive = !!batch && ['queued', 'running', 'paused'].includes(batch.status);
  const batchTerminal = !!batch && ['completed', 'failed', 'canceled'].includes(batch.status);

  // The rubric right panel is always visible when activeSub exists; no toggle needed.
  // activePartId drives which part's Popover is open in RubricPane.

  const filtered = useMemo(() => subs.filter(s => {
    if (filter === 'unreviewed') return s.status !== 'reviewed';
    if (filter === 'flagged')    return (s.result?.rubric_parts?.some(p => p.flags?.length)) || s.status === 'error';
    return true;
  }), [subs, filter]);

  /* ── derived scores ─────────────────────────────────────────── */
  const effectiveScore = draftParts.reduce((s, p) => s + (p.teacher_score != null ? p.teacher_score : p.ai_score), 0);
  const aiScore        = activeSub?.result?.ai_total_score ?? activeSub?.ai_score ?? 0;
  const hasOverride    = draftParts.some(p => p.teacher_score != null && p.teacher_score !== p.ai_score);
  const activeGrading  = !!activeSub && (gradingIds.includes(activeSub.id) || activeSub.status === 'grading' || activeSub.status === 'queued');

  /* ── handlers ───────────────────────────────────────────────── */
  const handleGrade = useCallback(async () => {
    if (!activeSub || gradingIds.includes(activeSub.id) || saving) return;
    const needsConfirm = activeSub.status === 'reviewed' || activeSub.status === 'needs_rereview';
    if (needsConfirm && !confirm('该答卷已核定。重跑会保留教师分项与批注，但会标记为“待复核”。继续？')) return;
    const hadDirty = draftDirty;
    setGradingIds(ids => [...ids, activeSub.id]);
    try {
      const updated = await api.gradeOne(activeSub.id, needsConfirm);
      onSubUpdated(updated);
      toast(hadDirty
        ? `${updated.student_name.trim()} AI 已更新；本地改分/批注已合并到新分项，请重新核定`
        : `${updated.student_name.trim()} AI 评阅完成`);
    } catch (e: unknown) {
      toast(e instanceof Error ? e.message : '评阅失败');
      onSubsRefresh().catch(() => undefined);
    } finally {
      setGradingIds(ids => ids.filter(id => id !== activeSub.id));
    }
  }, [activeSub, draftDirty, gradingIds, saving, onSubUpdated, onSubsRefresh, toast]);

  const handleGradeAll = async () => {
    if (batchActive || starting) { toast(batchActive ? '已有批改任务在进行中，请查看顶部进度' : '正在创建任务'); return; }
    setStarting(true);
    try {
      const r = await api.gradeAll(pset.id, true);
      if (r.task) {
        onBatchStarted(r.task);
        toast(r.reused ? '该题集已有批改任务，已接续显示进度' : `已创建批改任务：${r.queued} 份`);
        onSubsRefresh().catch(() => undefined);
      } else {
        toast(r.skipped > 0 ? `没有待批改答卷（已跳过 ${r.skipped} 份已评/已核定）` : '没有可批改的答卷');
      }
    } catch (e: unknown) {
      toast(e instanceof Error ? e.message : '批量评阅失败');
    } finally {
      setStarting(false);
    }
  };

  const handleCancelBatch = async () => {
    if (!batch) return;
    if (!confirm('确定取消当前批改任务？已完成的评分会保留。')) return;
    try { await onCancelBatch(batch.id); toast('已取消批改任务'); }
    catch (e: unknown) { toast(e instanceof Error ? e.message : '取消失败'); }
  };

  const handleResumeBatch = async () => {
    if (!batch) return;
    try { await onResumeBatch(batch.id); toast('已继续批改任务'); }
    catch (e: unknown) { toast(e instanceof Error ? e.message : '继续失败'); }
  };

  const handleExport = async () => {
    if (exporting) return;
    setExporting(true);
    try { await api.downloadCSV(pset.id); }
    catch (e: unknown) { toast(e instanceof Error ? e.message : '导出失败'); }
    finally { setExporting(false); }
  };

  const handleSaveAndNext = async () => {
    if (!activeSub || draftParts.length === 0 || saving || activeGrading) return;
    setSaving(true);
    let saved: Submission;
    try {
      const total = draftParts.reduce((s, p) => s + (p.teacher_score != null ? p.teacher_score : p.ai_score), 0);
      saved = await api.reviewSubmission(activeSub.id, total, draftNotes, draftParts);
      onSubUpdated(saved, true);
      toast(`${saved.student_name.trim()} 核定 ${total} 分`);
    } catch (e: unknown) {
      toast(e instanceof Error ? e.message : '保存失败');
      setSaving(false);
      return;
    }
    try {
      const latest = await onSubsRefresh();
      const idx = latest.findIndex(s => s.id === saved.id);
      if (idx >= 0) {
        const next =
          latest.slice(idx + 1).find(s => s.status !== 'reviewed' && s.status !== 'needs_rereview') ||
          latest[idx + 1];
        if (next) onSelectSub(next);
      }
    } catch {
      toast('已保存核定，但列表刷新失败，请手动刷新');
    } finally {
      setSaving(false);
    }
  };

  const selectSub = (sub: Submission) => {
    if (sub.id === activeSub?.id) return;
    if (draftDirty && !confirm('当前答卷有未核定的改分/批注，切换后会丢失。继续？')) {
      return;
    }
    onSelectSub(sub);
  };

  const handleBack = () => {
    if (draftDirty && !confirm('当前答卷有未核定的改分/批注，返回题库会丢失。继续？')) return;
    onBack();
  };

  const handleDelete = async (e: React.MouseEvent, id: number) => {
    e.stopPropagation();
    if (!confirm('确定移除这份答卷？')) return;
    try {
      await api.deleteSubmission(id);
      const latest = await onSubsRefresh();
      if (activeSub?.id === id) {
        const next = latest[0];
        if (next) onSelectSub(next);
      }
      toast('已移除答卷');
    } catch (err: unknown) {
      toast(err instanceof Error ? err.message : '删除失败');
    }
  };

  /* ── keyboard shortcuts ─────────────────────────────────────── */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      if (showImport || showShortcuts) return;
      const onButton = !!el?.closest('button, a, [role="button"]');
      if (onButton && (e.key === 'Enter' || e.key === ' ')) return;

      if (e.key === '?')      { e.preventDefault(); setShowShortcuts(v => !v); return; }
      if (e.key === 'Escape') { setShowShortcuts(false); return; }

      if (!e.ctrlKey && !e.metaKey && !e.altKey && (e.key === 'j' || e.key === 'ArrowDown')) {
        e.preventDefault();
        const idx = filtered.findIndex(s => s.id === activeSub?.id);
        if (idx >= 0 && idx < filtered.length - 1) selectSub(filtered[idx + 1]);
      }
      if (!e.ctrlKey && !e.metaKey && !e.altKey && (e.key === 'k' || e.key === 'ArrowUp')) {
        e.preventDefault();
        const idx = filtered.findIndex(s => s.id === activeSub?.id);
        if (idx > 0) selectSub(filtered[idx - 1]);
      }
      if (!e.ctrlKey && !e.metaKey && !e.altKey && e.key.toLowerCase() === 'g' && activeSub) {
        e.preventDefault();
        if (!batchActive) handleGrade();
      }
      if (e.key === 'Enter' && !e.shiftKey && draftParts.length && activeSub) {
        e.preventDefault();
        if (!saving) handleSaveAndNext();
      }
      if (e.altKey && ['0', '1', '2'].includes(e.key) && activePartId) {
        e.preventDefault(); onSetPartScore(activePartId, Number(e.key));
      }
      if (!e.altKey && ['1', '2', '3', '4', '5', '6'].includes(e.key) && draftParts[Number(e.key) - 1]) {
        e.preventDefault(); onSetActivePartId(draftParts[Number(e.key) - 1].part_id);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSub, draftParts, draftNotes, activePartId, filtered, showImport, showShortcuts, draftDirty, saving, gradingIds, batchActive]);


  return (
    <div className="h-screen flex flex-col bg-[#F6F5F1] overflow-hidden">

      {/* ── header ──────────────────────────────────────────────── */}
      <header className="h-12 shrink-0 bg-white border-b border-[#E5E2DA] flex items-center justify-between px-4 gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <button onClick={handleBack} className="p-1.5 rounded-lg hover:bg-[#F2F0EA] text-[#57544E]" title="返回题库">
            <ArrowLeft className="w-4 h-4" />
          </button>
          <div className="flex items-center gap-2 min-w-0">
            <span className="w-1.5 h-1.5 rounded-full bg-[#1F4E3B] shrink-0" />
            <span className="text-sm font-medium text-[#191918] truncate">{pset.title}</span>
            <span className={`text-[10px] font-semibold px-1.5 py-0.5 rounded shrink-0 ${
              pset.question_type === 'EBQ' ? 'bg-[#FEF3C7] text-[#92400E]' : 'bg-[#E0F2FE] text-[#075985]'
            }`}>{pset.question_type}</span>
          </div>
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <button
            onClick={() => setShowPrompt(v => !v)}
            className={`px-2.5 py-1.5 rounded-lg text-xs font-medium transition ${
              showPrompt ? 'bg-[#191918] text-white' : 'text-[#57544E] hover:bg-[#F2F0EA]'
            }`}
          >题干</button>
          <button
            onClick={handleGradeAll}
            disabled={subs.length === 0 || batchActive || starting}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-medium text-[#3F3D39] hover:bg-[#F2F0EA] disabled:opacity-40"
          >
            <Sparkles className="w-3.5 h-3.5 text-[#92400E]" /> {starting ? '创建中…' : '全班批改'}
          </button>
          <button
            onClick={handleExport}
            disabled={exporting}
            className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-medium text-[#3F3D39] hover:bg-[#F2F0EA] disabled:opacity-40"
          >
            <FileSpreadsheet className="w-3.5 h-3.5" /> {exporting ? '导出中…' : '导出'}
          </button>
          <button onClick={() => setShowShortcuts(true)} className="p-1.5 rounded-lg text-[#57544E] hover:bg-[#F2F0EA]">
            <HelpCircle className="w-4 h-4" />
          </button>
        </div>
      </header>

      {/* ── batch progress ─────────────────────────────────────── */}
      {batch && (
        <div className={`shrink-0 border-b px-4 py-2 flex items-center gap-3 text-xs ${
          batch.status === 'paused' ? 'bg-[#FFFBEB] border-[#FDE68A]' :
          batch.status === 'failed' ? 'bg-[#FFE4E6] border-[#FECDD3]' :
          batchTerminal ? 'bg-[#F0FDF4] border-[#BBF7D0]' : 'bg-[#FAF9F5] border-[#E5E2DA]'
        }`}>
          {batchActive && <Loader2 className={`w-3.5 h-3.5 shrink-0 ${batch.status === 'running' ? 'animate-spin text-[#92400E]' : 'text-[#A39F96]'}`} />}
          {batchTerminal && <CheckCircle2 className={`w-3.5 h-3.5 shrink-0 ${batch.status === 'completed' ? 'text-[#15803D]' : 'text-[#9F1239]'}`} />}
          <span className="font-medium text-[#191918] shrink-0">
            {batch.status === 'queued' ? '排队中' :
             batch.status === 'running' ? 'AI 批改中' :
             batch.status === 'paused' ? '已暂停' :
             batch.status === 'canceled' ? '已取消' :
             batch.status === 'failed' ? '批改失败' :
             `批改完成${batch.failed > 0 ? `（${batch.failed} 份失败）` : ''}`}
          </span>
          <div className="w-40 h-1.5 rounded-full bg-[#E5E2DA] overflow-hidden shrink-0">
            <div
              className={`h-full rounded-full transition-all ${batch.status === 'failed' ? 'bg-[#BE123C]' : batch.status === 'paused' ? 'bg-[#D97706]' : 'bg-[#1F4E3B]'}`}
              style={{ width: `${batch.total ? Math.round(((batch.succeeded + batch.failed + batch.canceled) / batch.total) * 100) : 0}%` }}
            />
          </div>
          <span className="tabular-nums text-[#57544E] shrink-0">
            {batch.succeeded + batch.failed + batch.canceled}/{batch.total} · 成功 {batch.succeeded}
            {batch.failed > 0 ? ` · 失败 ${batch.failed}` : ''}
            {batch.canceled > 0 ? ` · 跳过 ${batch.canceled}` : ''}
          </span>
          {batch.current.length > 0 && (
            <span className="truncate text-[#78756E]">
              正在：{batch.current.slice(0, 3).map(i => i.student_name.trim()).filter(Boolean).join('、')}
              {batch.current.length > 3 ? ` 等 ${batch.current.length} 人` : ''}
            </span>
          )}
          <div className="ml-auto flex items-center gap-1.5 shrink-0">
            {(batch.status === 'paused' || batch.status === 'failed') && (
              <button onClick={handleResumeBatch} className="px-2 py-1 rounded-lg bg-[#191918] text-white text-[11px] font-medium">
                继续批改
              </button>
            )}
            {batchActive && (
              <button onClick={handleCancelBatch} className="px-2 py-1 rounded-lg border border-[#DCD9D0] text-[11px] font-medium text-[#57544E] hover:bg-white">
                取消
              </button>
            )}
            {batchTerminal && batch.failed > 0 && (
              <button onClick={handleGradeAll} className="px-2 py-1 rounded-lg bg-[#191918] text-white text-[11px] font-medium">
                重试失败项
              </button>
            )}
            {batchTerminal && (
              <button onClick={onDismissBatch} className="px-2 py-1 rounded-lg border border-[#DCD9D0] text-[11px] font-medium text-[#57544E] hover:bg-white">
                关闭
              </button>
            )}
          </div>
        </div>
      )}

      {/* ── prompt drawer ───────────────────────────────────────── */}
      <AnimatePresence>
        {showPrompt && (
          <motion.div
            initial={{ height: 0 }} animate={{ height: 'auto' }} exit={{ height: 0 }}
            className="overflow-hidden bg-[#FAF8F3] border-b border-[#E5E2DA] shrink-0"
          >
            <div className="px-6 py-3 max-w-3xl text-xs text-[#45433E] leading-relaxed select-text">
              <p className="font-medium text-[#191918] mb-1">题干</p>
              <p>{pset.prompt_text}</p>
              {pset.sources?.length > 0 && (
                <div className="mt-2 space-y-1">
                  {pset.sources.map(src => (
                    <div key={src.id}>
                      <span className="font-medium text-[#191918]">{src.label} · {src.title}：</span>
                      {src.summary}
                    </div>
                  ))}
                </div>
              )}
              {pset.ineligible_concepts?.length ? (
                <div className="mt-2 flex flex-wrap items-center gap-1">
                  <span className="font-medium text-[#9F1239]">禁用词：</span>
                  {pset.ineligible_concepts.map(t => (
                    <span key={t} className="font-mono text-[#9F1239] bg-[#FFE4E6] px-1.5 py-0.5 rounded text-[11px]">{t}</span>
                  ))}
                </div>
              ) : null}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* ── body ────────────────────────────────────────────────── */}
      <div className="flex-1 min-h-0 relative flex">

        {/* ── left queue ────────────────────────────────────────── */}
        <AnimatePresence initial={false}>
          {queueOpen ? (
            <motion.aside
              key="queue-open"
              initial={{ width: 0, opacity: 0 }}
              animate={{ width: 216, opacity: 1 }}
              exit={{ width: 0, opacity: 0 }}
              transition={{ type: 'spring', stiffness: 300, damping: 30 }}
              className="shrink-0 overflow-hidden border-r border-[#E5E2DA] bg-white flex flex-col z-10"
            >
              <div className="w-[216px] h-full flex flex-col">
                {/* filter tabs + collapse */}
                <div className="px-2 py-2 border-b border-[#ECE9E1] flex items-center gap-1.5">
                  <div className="flex-1 grid grid-cols-3 gap-0.5 rounded-lg bg-[#F2F0EA] p-0.5 text-[10.5px]">
                    {([['all','全部'],['unreviewed','待核'],['flagged','AI 提示']] as const).map(([k, l]) => (
                      <button
                        key={k}
                        onClick={() => setFilter(k)}
                        className={`py-1 rounded-md transition ${
                          filter === k ? 'bg-white text-[#191918] font-medium shadow-sm' : 'text-[#6E6B65] hover:text-[#191918]'
                        }`}
                      >{l}</button>
                    ))}
                  </div>
                  <button
                    onClick={() => setQueueOpen(false)}
                    className="p-1 rounded-lg hover:bg-[#F2F0EA] text-[#B8B4AA] hover:text-[#57544E] shrink-0"
                    title="折叠队列"
                  >
                    <PanelLeft className="w-3.5 h-3.5" />
                  </button>
                </div>

                {/* student list */}
                <div className="flex-1 min-h-0 overflow-y-auto p-1.5 space-y-0.5">
                  {filtered.map(s => {
                    const sel  = s.id === activeSub?.id;
                    const sc   = s.final_score ?? s.ai_score;
                    const warn = s.result?.rubric_parts?.some(p => p.flags?.length);
                    const dot  = s.status === 'error' ? 'bg-[#BE123C]'
                      : s.status === 'needs_rereview' ? 'bg-[#D97706]'
                      : s.status === 'canceled' ? 'bg-[#A39F96]'
                      : warn ? 'bg-[#D97706]' : 'bg-[#B8B4AA]';
                    return (
                      <div
                        key={s.id}
                        onClick={() => selectSub(s)}
                        title={s.status === 'needs_rereview' ? 'AI 已更新，待复核' : s.status}
                        className={`group flex items-center gap-2 px-2.5 py-2 rounded-lg cursor-pointer transition select-none ${
                          sel ? 'bg-[#F2EFE8]' : 'hover:bg-[#FAF9F5]'
                        }`}
                      >
                        {s.status === 'grading'
                          ? <Loader2 className="w-3.5 h-3.5 text-[#92400E] animate-spin shrink-0" />
                          : s.status === 'queued'
                            ? <Loader2 className="w-3.5 h-3.5 text-[#A39F96] animate-pulse shrink-0" />
                            : s.status === 'reviewed'
                              ? <CheckCircle2 className="w-3.5 h-3.5 text-[#15803D] shrink-0" />
                              : <span className={`w-2 h-2 rounded-full shrink-0 ${dot}`} />
                        }
                        <span className="text-xs font-medium truncate text-[#191918] flex-1 min-w-0">{s.student_name.trim()}</span>
                        {sc != null && (
                          <span className={`text-[11px] font-semibold tabular-nums shrink-0 ${s.status === 'reviewed' ? 'text-[#15803D]' : 'text-[#A39F96]'}`}>
                            {sc}/{s.result?.max_total_score ?? 7}
                          </span>
                        )}
                        <button
                          onClick={e => handleDelete(e, s.id)}
                          className="opacity-0 group-hover:opacity-100 text-[#B8B4AA] hover:text-[#9F1239] shrink-0 transition-opacity"
                        >
                          <X className="w-3 h-3" />
                        </button>
                      </div>
                    );
                  })}
                  {filtered.length === 0 && (
                    <div className="py-8 text-center text-[11px] text-[#A39F96]">暂无答卷</div>
                  )}
                </div>

                {/* bottom: import + grade-all */}
                <div className="p-2 border-t border-[#ECE9E1] space-y-1.5">
                  <button
                    onClick={() => setShowImport(true)}
                    className="w-full inline-flex items-center justify-center gap-1.5 py-2 rounded-lg bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A]"
                  >
                    <FilePlus2 className="w-3.5 h-3.5" /> 导入答卷
                  </button>
                  <button
                    onClick={handleGradeAll}
                    disabled={subs.length === 0 || batchActive || starting}
                    className="w-full inline-flex items-center justify-center gap-1.5 py-2 rounded-lg border border-[#DCD9D0] text-xs font-medium text-[#3F3D39] hover:bg-[#FAF9F5] disabled:opacity-40"
                  >
                    <Sparkles className="w-3.5 h-3.5 text-[#92400E]" /> {starting ? '创建中…' : '一键批改'}
                  </button>
                </div>
              </div>
            </motion.aside>
          ) : (
            /* collapsed: floating pill on left edge */
            <motion.div
              key="queue-pill"
              initial={{ opacity: 0, x: -8 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -8 }}
              className="absolute left-0 top-1/2 -translate-y-1/2 z-20"
            >
              <button
                onClick={() => setQueueOpen(true)}
                className="flex flex-col items-center justify-center w-6 h-16 bg-white border border-[#E5E2DA] rounded-r-xl shadow-sm hover:bg-[#F2F0EA] transition text-[#78756E] hover:text-[#191918]"
                title="展开学生队列"
              >
                <ChevronRight className="w-3.5 h-3.5" />
                <span className="text-[9px] font-medium mt-0.5 [writing-mode:vertical-rl] tracking-wide">
                  {filtered.length}
                </span>
              </button>
            </motion.div>
          )}
        </AnimatePresence>

        {/* ── paper (center) ──────────────────────────────────────── */}
        <main className="flex-1 min-w-0 h-full min-h-0 overflow-y-auto">
          {activeSub ? (
            <PaperPane
              sub={activeSub}
              activePartId={activePartId}
              onSelectPart={onSetActivePartId}
              onGrade={handleGrade}
              grading={activeGrading}
            />
          ) : (
            <div className="h-full flex flex-col items-center justify-center text-center gap-3">
              <FilePlus2 className="w-8 h-8 text-[#B8B4AA]" />
              <p className="text-sm font-medium text-[#3F3D39]">还没有学生答卷</p>
              <button
                onClick={() => setShowImport(true)}
                className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A]"
              >
                <Upload className="w-3.5 h-3.5" /> 导入 PDF
              </button>
            </div>
          )}
        </main>

        {/* ── rubric rail (right) ──────────────────────────────────── */}
        {activeSub && (
          <aside className="w-[280px] shrink-0 border-l border-[#E5E2DA] bg-white flex flex-col overflow-visible">
            <RubricPane
              sub={activeSub}
              parts={draftParts}
              notes={draftNotes}
              activePartId={activePartId}
              effectiveScore={effectiveScore}
              aiScore={aiScore}
              hasOverride={hasOverride}
              saving={saving}
              grading={activeGrading}
              onSelectPart={onSetActivePartId}
              onSetScore={onSetPartScore}
              onReset={onResetToAI}
              onNotes={onSetNotes}
              onSave={handleSaveAndNext}
              onGrade={handleGrade}
            />
          </aside>
        )}
      </div>

      {/* ── modals ──────────────────────────────────────────────── */}
      {showImport && (
        <ImportModal
          psetId={pset.id}
          psetTitle={pset.title}
          onClose={() => setShowImport(false)}
          onDone={(task, warning) => {
            setShowImport(false);
            if (task) onBatchStarted(task);
            if (warning) toast(warning);
            onSubsRefresh().catch(() => undefined);
          }}
        />
      )}
      {showShortcuts && <ShortcutsModal onClose={() => setShowShortcuts(false)} />}
    </div>
  );
};

/* ══════════════════════════════════════════════════════════════════
   PaperPane
   Left column = English sentence, right column = Chinese translation.
   Both grow from the same grid row; no vertical stacking.
   ══════════════════════════════════════════════════════════════════ */
const PaperPane: React.FC<{
  sub: Submission;
  activePartId: string | null;
  onSelectPart: (id: string | null) => void;
  onGrade: () => void;
  grading: boolean;
}> = ({ sub, activePartId, onSelectPart, onGrade, grading }) => {
  const [showZh, setShowZh] = useState(true);
  const result = sub.result;

  /* render sentence text with per-span colour highlighting (no badges, no overlays) */
  const renderSentence = (sentId: string, text: string): React.ReactNode => {
    if (!result) return text;
    const anns = (result.annotations ?? []).filter(a => a.sentence_id === sentId);
    if (anns.length === 0) return text;

    type Sp = { start: number; end: number; ann: Annotation };
    const spans: Sp[] = [];
    const lower = text.toLowerCase();

    for (const ann of anns) {
      const q = (ann.exact_quote || '').trim().toLowerCase();
      let start = -1, end = -1;
      if (q) { const i = lower.indexOf(q); if (i !== -1) { start = i; end = i + q.length; } }
      if (start === -1 && spans.length === 0) { start = 0; end = text.length; }
      if (start !== -1 && end > start) {
        const overlap = spans.some(s => !(end <= s.start || start >= s.end));
        if (!overlap) spans.push({ start, end, ann });
      }
    }
    if (spans.length === 0) return text;
    spans.sort((a, b) => a.start - b.start);

    const nodes: React.ReactNode[] = [];
    let cursor = 0;
    spans.forEach((sp, i) => {
      if (sp.start > cursor) nodes.push(<span key={`pre${i}`}>{text.slice(cursor, sp.start)}</span>);
      const st = CAT_STYLE[sp.ann.category] || CAT_STYLE.issue;
      const focused = activePartId === sp.ann.part_id;
      nodes.push(
        <mark
          key={`h${sp.ann.id}`}
          onClick={e => { e.stopPropagation(); onSelectPart(sp.ann.part_id); }}
          className={`cursor-pointer rounded-[2px] transition-all ${st.bg} ${focused ? st.ring : ''}`}
          style={{ padding: '0 1px' }}
        >
          {text.slice(sp.start, sp.end)}
        </mark>
      );
      cursor = sp.end;
    });
    if (cursor < text.length) nodes.push(<span key="tail">{text.slice(cursor)}</span>);
    return <>{nodes}</>;
  };

  /* group sentences into paragraphs aligned with raw_text paragraph breaks */
  const paragraphs = useMemo(() => {
    if (!result || (result.sentences ?? []).length === 0) {
      return sub.raw_text.split(/\n\s*\n/).filter(p => p.trim()).map(p => [{ id: '', en: p, zh: '' }]);
    }
    const sents = result.sentences ?? [];
    const rawParas = sub.raw_text.split(/\n\s*\n/).filter(p => p.trim());
    const grouped: { id: string; en: string; zh: string }[][] = [];
    let si = 0;
    for (const para of rawParas) {
      const bucket: { id: string; en: string; zh: string }[] = [];
      while (si < sents.length) {
        const s = sents[si];
        if (s.en.trim() && para.replace(/\s+/g, ' ').includes(s.en.replace(/\s+/g, ' ').slice(0, 40))) {
          bucket.push({ id: s.id, en: s.en, zh: s.zh });
          si++;
        } else break;
      }
      if (bucket.length === 0 && si < sents.length) {
        const s = sents[si++];
        bucket.push({ id: s.id, en: s.en, zh: s.zh });
      }
      grouped.push(bucket);
    }
    if (si < sents.length) grouped.push(sents.slice(si).map(s => ({ id: s.id, en: s.en, zh: s.zh })));
    return grouped;
  }, [sub.raw_text, result]);

  return (
    <div className="max-w-4xl mx-auto px-6 py-7">
      {/* student meta row */}
      <div className="flex items-center justify-between mb-5">
        <div className="flex items-baseline gap-2">
          <h2 className="text-base font-semibold text-[#191918]">{sub.student_name.trim()}</h2>
          {sub.student_code && <span className="text-xs font-mono text-[#A39F96]">{sub.student_code}</span>}
          <span className="text-xs text-[#B8B4AA] tabular-nums">
            {sub.raw_text.trim().split(/\s+/).filter(Boolean).length} 词
          </span>
        </div>
        <div className="flex items-center gap-2">
          {result && (
            <button
              onClick={() => setShowZh(v => !v)}
              className={`px-2.5 py-1 rounded-lg text-xs font-medium transition ${
                showZh ? 'bg-[#F2F0EA] text-[#191918]' : 'text-[#A39F96] hover:text-[#191918]'
              }`}
            >中文释义</button>
          )}
          <button
            onClick={onGrade}
            disabled={grading}
            className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg border border-[#DCD9D0] bg-white text-xs font-medium text-[#3F3D39] hover:bg-[#FAF9F6] disabled:opacity-50"
          >
            {grading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <RotateCcw className="w-3.5 h-3.5" />}
            {grading ? '评阅中' : '重跑 (G)'}
          </button>
        </div>
      </div>

      {/* status banners (visible even when an old result exists) */}
      {sub.status === 'error' && sub.error_msg && (
        <div className="mb-5 flex items-start gap-2 text-xs text-[#9F1239] bg-[#FFE4E6] rounded-xl px-3 py-2">
          <AlertTriangle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
          <span className="flex-1 leading-relaxed">{sub.error_msg}</span>
          <button onClick={onGrade} disabled={grading} className="font-medium underline shrink-0 disabled:opacity-50">重试</button>
        </div>
      )}
      {sub.status === 'needs_rereview' && (
        <div className="mb-5 flex items-start gap-2 text-xs text-[#92400E] bg-[#FEF3C7] rounded-xl px-3 py-2">
          <AlertTriangle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
          <span className="leading-relaxed">AI 结果已更新；教师逐项分与批注仍保留。请检查后重新「核定」。</span>
        </div>
      )}
      {sub.status === 'queued' && (
        <div className="mb-5 flex items-center gap-2 text-xs text-[#57544E] bg-[#F2F0EA] rounded-xl px-3 py-2">
          <Loader2 className="w-3.5 h-3.5 animate-pulse" /> 已排队，等待 AI 批改 worker 领取。
        </div>
      )}

      {/* colour legend */}
      {result && (
        <div className="flex flex-wrap items-center gap-3 mb-5 text-[11px] text-[#78756E]">
          {(Object.keys(CAT_STYLE) as AnnotationCategory[]).map(cat => {
            const n = (result.annotations ?? []).filter(a => a.category === cat).length;
            if (n === 0) return null;
            return (
              <span key={cat} className="inline-flex items-center gap-1">
                <span className={`w-2 h-2 rounded-full shrink-0 ${CAT_STYLE[cat].dot}`} />
                {CAT_STYLE[cat].label}
              </span>
            );
          })}
        </div>
      )}

      {/* essay — two columns: EN left (dominant) + ZH right (muted, smaller) */}
      {result ? (
        <div className="space-y-5">
          {paragraphs.map((para, pi) => (
            <div key={pi} className="space-y-2.5">
              {para.map((s, si) => (
                <div key={si} className={`${showZh && s.zh ? 'grid gap-x-5' : ''}`}
                  style={showZh && s.zh ? { gridTemplateColumns: '1fr 0.6fr' } : undefined}
                >
                  {/* English */}
                  <p className="leading-[1.9] text-[15px] text-[#2A2926] m-0">
                    {renderSentence(s.id, s.en)}
                  </p>
                  {/* Chinese — only when toggle on and zh available */}
                  {showZh && s.zh && (
                    <p className="leading-[1.75] text-[13px] text-[#8C8880] m-0 pt-[2px]">
                      {s.zh}
                    </p>
                  )}
                </div>
              ))}
            </div>
          ))}
        </div>
      ) : (
        <div className="space-y-4">
          <pre className="leading-[1.9] text-[15px] text-[#2A2926] whitespace-pre-wrap font-sans m-0">{sub.raw_text}</pre>
          <button
            onClick={onGrade}
            disabled={grading}
            className="inline-flex items-center gap-2 px-4 py-2.5 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A] disabled:opacity-50"
          >
            {grading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Sparkles className="w-4 h-4 text-[#FCD34D]" />}
            {grading ? 'AI 评阅中…' : '运行 AI 评阅'}
          </button>
        </div>
      )}

      {result?.summary_zh && (
        <div className="mt-7 pt-5 border-t border-[#EAE7DF] text-[13px] text-[#57544E] leading-relaxed">
          <span className="font-medium text-[#191918]">AI 总评：</span>{result.summary_zh}
        </div>
      )}
    </div>
  );
};

/* ══════════════════════════════════════════════════════════════════
   RubricPane
   Compact one-line rows.
   Detail (reason, source, flags) opens as a Popover to the left of the row.
   Score pills remain always-visible and clickable.
   ══════════════════════════════════════════════════════════════════ */
/* ── PartPopover: portalled into document.body to escape overflow clipping ── */
const PartPopover: React.FC<{
  anchorRef: React.RefObject<HTMLDivElement | null>;
  part: RubricPartScore;
  onClose: () => void;
  onSetScore: (partId: string, score: number) => void;
  onMouseEnter?: () => void;
  onMouseLeave?: () => void;
}> = ({ anchorRef, part, onClose, onSetScore, onMouseEnter, onMouseLeave }) => {
  const [pos, setPos] = useState({ top: 0, right: 0 });

  useEffect(() => {
    const update = () => {
      if (!anchorRef.current) return;
      const r = anchorRef.current.getBoundingClientRect();
      setPos({ top: r.top, right: window.innerWidth - r.left + 8 });
    };
    update();
    window.addEventListener('scroll', update, true);
    window.addEventListener('resize', update);
    return () => {
      window.removeEventListener('scroll', update, true);
      window.removeEventListener('resize', update);
    };
  }, [anchorRef]);


  const cur = part.teacher_score != null ? part.teacher_score : part.ai_score;
  const hasFlags = !!(part.flags && part.flags.length > 0);

  return createPortal(
    <motion.div
      id="part-popover-portal"
      key={part.part_id}
      initial={{ opacity: 0, x: 8, scale: 0.97 }}
      animate={{ opacity: 1, x: 0, scale: 1 }}
      exit={{ opacity: 0, x: 8, scale: 0.97 }}
      transition={{ duration: 0.14 }}
      style={{ position: 'fixed', top: pos.top, right: pos.right, zIndex: 9999 }}
      className="w-[340px] bg-white rounded-2xl border border-[#E2DFD7] shadow-xl p-4 space-y-3 text-[12px]"
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="font-semibold text-[#191918] text-[13px]">{part.label}</div>
          {part.applied_concept && (
            <div className="text-[11px] text-[#78756E] mt-0.5">{part.applied_concept}</div>
          )}
        </div>
        <button onClick={onClose} className="p-0.5 rounded-lg hover:bg-[#F2F0EA] text-[#B8B4AA] shrink-0">
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      {part.cited_source && (
        <div className="flex gap-2 items-start">
          <span className="text-[#A39F96] shrink-0 mt-0.5">出处</span>
          <span className="text-[#3F3D39] leading-relaxed">{part.cited_source}</span>
        </div>
      )}

      {part.reason_zh && (
        <div className="text-[#45433E] leading-relaxed bg-[#FAF9F6] rounded-xl px-3 py-2.5">
          {part.reason_zh}
        </div>
      )}

      {hasFlags && (
        <div className="space-y-1.5">
          <div className="text-[10.5px] font-semibold text-[#78756E] uppercase tracking-wider">AI 提示</div>
          {part.flags!.map(f => {
            const meta = FLAG_LABELS[f] || { text: f, tone: 'warn' as const };
            return (
              <div key={f} className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-lg text-[11px] mr-1 ${
                meta.tone === 'err' ? 'bg-[#FFE4E6] text-[#9F1239]' : 'bg-[#FEF3C7] text-[#92400E]'
              }`}>
                <AlertTriangle className="w-2.5 h-2.5 shrink-0" />{meta.text}
              </div>
            );
          })}
        </div>
      )}

      <div className="flex items-center gap-2 pt-1 border-t border-[#F0EEE8]">
        <span className="text-[#A39F96] text-[11px]">改分</span>
        <div className="inline-flex rounded-lg bg-[#F2F0EA] p-0.5">
          {Array.from({ length: part.max_score + 1 }, (_, v) => (
            <button
              key={v}
              onClick={() => onSetScore(part.part_id, v)}
              className={`px-2.5 py-1 text-[12px] font-semibold rounded-md tabular-nums transition ${
                cur === v
                  ? v === part.max_score ? 'bg-[#1F4E3B] text-white' : v > 0 ? 'bg-[#85581A] text-white' : 'bg-[#64615A] text-white'
                  : 'text-[#78756E] hover:text-[#191918]'
              }`}
            >{v}</button>
          ))}
        </div>
        <span className="text-[10px] text-[#B8B4AA] ml-auto">/ {part.max_score}</span>
      </div>
    </motion.div>,
    document.body,
  );
};

const RubricPane: React.FC<{
  sub: Submission;
  parts: RubricPartScore[];
  notes: string;
  activePartId: string | null;
  effectiveScore: number;
  aiScore: number;
  hasOverride: boolean;
  saving: boolean;
  grading: boolean;
  onSelectPart: (id: string | null) => void;
  onSetScore: (id: string, score: number) => void;
  onReset: () => void;
  onNotes: (n: string) => void;
  onSave: () => void;
  onGrade: () => void;
}> = ({
  parts, notes, activePartId, effectiveScore, aiScore, hasOverride,
  saving, grading, onSelectPart, onSetScore, onReset, onNotes, onSave,
}) => {
  const [popover, setPopover] = useState<string | null>(null);
  const rowRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const hoverTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const maxTotal = parts.reduce((sum, p) => sum + p.max_score, 0) || 7;

  const openPopover = (partId: string) => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current);
    setPopover(partId);
    onSelectPart(partId);
  };
  const closePopoverDelayed = () => {
    hoverTimerRef.current = setTimeout(() => setPopover(null), 200);
  };
  const cancelClose = () => {
    if (hoverTimerRef.current) clearTimeout(hoverTimerRef.current);
  };

  return (
    <div className="w-full h-full flex flex-col overflow-visible">
      {/* score header */}
      <div className="px-4 py-3 border-b border-[#ECE9E1] flex items-center justify-between shrink-0">
        <div>
          <div className="text-[10px] font-semibold text-[#A39F96] uppercase tracking-wider mb-0.5">总分</div>
          <div className="flex items-baseline gap-1.5">
            <span className="text-2xl font-semibold tabular-nums text-[#191918]">
              {parts.length ? effectiveScore : '—'}
            </span>
            <span className="text-xs text-[#A39F96]">/ {maxTotal}</span>
            {hasOverride && (
              <span className="text-[10px] text-[#92400E] ml-1">AI {aiScore}</span>
            )}
          </div>
        </div>
        <button
          onClick={onSave}
          disabled={saving || grading || parts.length === 0}
          className="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A] disabled:opacity-40"
        >
          <Check className="w-3.5 h-3.5" />
          核定
          <kbd className="ml-0.5 px-1 py-0 rounded bg-white/15 text-[10px] font-mono">↵</kbd>
        </button>
      </div>

      {/* parts list */}
      <div className="flex-1 min-h-0 overflow-y-auto">
        {parts.length === 0 ? (
          <div className="py-10 text-center text-[11px] text-[#A39F96] px-4">
            {grading ? 'AI 评阅中…' : '点击「重跑」生成逐项采分'}
          </div>
        ) : (
          parts.map((p, i) => {
            const cur      = p.teacher_score != null ? p.teacher_score : p.ai_score;
            const isActive = activePartId === p.part_id;
            const hasFlags = p.flags && p.flags.length > 0;
            const isOpen   = popover === p.part_id;

            return (
              <div
                key={p.part_id}
                ref={el => { if (el) rowRefs.current.set(p.part_id, el); else rowRefs.current.delete(p.part_id); }}
                className={`relative border-b border-[#F0EEE8] ${isActive ? 'bg-[#FAF9F5]' : ''}`}
              >
                {/* one-line row */}
                <div className="flex items-center gap-2 px-3 py-2.5">
                  <span className="text-[10px] font-mono text-[#C4C0B8] w-3.5 shrink-0 select-none">{i + 1}</span>

                  {/* label — hover opens popover */}
                  <button
                    onMouseEnter={() => openPopover(p.part_id)}
                    onMouseLeave={closePopoverDelayed}
                    className="flex-1 min-w-0 text-left"
                  >
                    <div className="text-xs font-medium text-[#191918] truncate">{p.label}</div>
                    {p.applied_concept && (
                      <div className="text-[10.5px] text-[#78756E] truncate">{p.applied_concept}</div>
                    )}
                  </button>

                  {/* AI-提示 indicator */}
                  {hasFlags && (
                    <Info className="w-3 h-3 text-[#D97706] shrink-0" />
                  )}

                  {/* score pills */}
                  <div
                    className="inline-flex rounded-lg bg-[#F2F0EA] p-0.5 shrink-0"
                    onClick={e => e.stopPropagation()}
                  >
                    {Array.from({ length: p.max_score + 1 }, (_, v) => (
                      <button
                        key={v}
                        onClick={() => onSetScore(p.part_id, v)}
                        className={`px-2 py-0.5 text-[11px] font-semibold rounded-md tabular-nums transition ${
                          cur === v
                            ? v === p.max_score
                              ? 'bg-[#1F4E3B] text-white'
                              : v > 0
                                ? 'bg-[#85581A] text-white'
                                : 'bg-[#64615A] text-white'
                            : 'text-[#78756E] hover:text-[#191918]'
                        }`}
                      >{v}</button>
                    ))}
                  </div>
                </div>

                {isOpen && rowRefs.current.get(p.part_id) && (
                  <PartPopover
                    anchorRef={{ current: rowRefs.current.get(p.part_id) ?? null }}
                    part={p}
                    onClose={() => setPopover(null)}
                    onSetScore={onSetScore}
                    onMouseEnter={cancelClose}
                    onMouseLeave={closePopoverDelayed}
                  />
                )}
              </div>
            );
          })
        )}
      </div>

      {/* footer */}
      {parts.length > 0 && (
        <div className="px-4 py-3 border-t border-[#ECE9E1] space-y-2 shrink-0">
          <textarea
            rows={1}
            value={notes}
            onChange={e => onNotes(e.target.value)}
            placeholder="教师批注（可选）"
            className="w-full rounded-lg border border-[#E2DFD7] bg-[#FAF9F6] px-2.5 py-1.5 text-[11px] resize-none focus:outline-none focus:border-[#191918]"
          />
          {hasOverride && (
            <button
              onClick={onReset}
              className="inline-flex items-center gap-1 text-[10.5px] text-[#78756E] hover:text-[#191918]"
            >
              <RotateCcw className="w-3 h-3" /> 恢复 AI 建议分
            </button>
          )}
        </div>
      )}
    </div>
  );
};

/* ══════════════════════════════════════════════════════════════════
   ImportModal
   ══════════════════════════════════════════════════════════════════ */
const ImportModal: React.FC<{
  psetId: number; psetTitle: string; onClose: () => void; onDone: (task?: GradingTask, warning?: string) => void;
}> = ({ psetId, psetTitle, onClose, onDone }) => {
  const [file,       setFile]      = useState<File | null>(null);
  const [mode,       setMode]      = useState<'auto' | 'pages_per_student'>('auto');
  const [pps,        setPps]       = useState(1);
  const [segs,       setSegs]      = useState<SplitSegment[]>([]);
  const [pages,      setPages]     = useState(0);
  const [paste,      setPaste]     = useState('');
  const [autoGrade,  setAutoGrade] = useState(true);
  const [busy,       setBusy]      = useState(false);
  const [err,        setErr]       = useState('');
  const parseSeq = useRef(0);

  const parsePdf = async (f: File, m = mode, p = pps) => {
    const seq = ++parseSeq.current;
    setBusy(true); setErr('');
    setSegs([]); setPages(0);
    try {
      const r = await api.parsePDF(psetId, f, m, p);
      if (seq !== parseSeq.current) return;
      setPages(r.page_count); setSegs(r.segments || []);
    } catch (e: unknown) {
      if (seq !== parseSeq.current) return;
      setErr(e instanceof Error ? e.message : '解析失败');
    } finally {
      if (seq === parseSeq.current) setBusy(false);
    }
  };

  const onFile = (f: File | null) => { setFile(f); if (f) parsePdf(f); };

  const mergePrev = (i: number) => {
    if (i <= 0) return;
    setSegs(prev => {
      const c = [...prev];
      const a = c[i - 1], b = c[i];
      const txt = `${a.raw_text}\n\n${b.raw_text}`.trim();
      c[i - 1] = { ...a, page_range: `${a.page_range}, ${b.page_range}`, raw_text: txt, word_count: txt.split(/\s+/).filter(Boolean).length };
      c.splice(i, 1);
      return c.map((s, j) => ({ ...s, index: j + 1 }));
    });
  };

  const splitHalf = (i: number) => {
    setSegs(prev => {
      const c = [...prev];
      const cur = c[i];
      const paras = cur.raw_text.split(/\n\s*\n/).filter(p => p.trim());
      if (paras.length < 2) return prev;
      const mid = Math.ceil(paras.length / 2);
      const fa = paras.slice(0, mid).join('\n\n');
      const fb = paras.slice(mid).join('\n\n');
      c.splice(i, 1,
        { ...cur, raw_text: fa, word_count: fa.split(/\s+/).filter(Boolean).length },
        { ...cur, index: cur.index + 1, student_name: `${cur.student_name} (Part 2)`, raw_text: fb, word_count: fb.split(/\s+/).filter(Boolean).length }
      );
      return c.map((x, j) => ({ ...x, index: j + 1 }));
    });
  };

  const removeSeg = (i: number) =>
    setSegs(prev => prev.filter((_, j) => j !== i).map((s, j) => ({ ...s, index: j + 1 })));

  const fromPaste = () => {
    const blocks = paste.split(/\n\s*---+\s*\n/).map(b => b.trim()).filter(Boolean);
    setSegs(blocks.map((blk, i) => {
      const lines = blk.split('\n');
      let name = `Student #${String(i + 1).padStart(2, '0')}`;
      let body = blk;
      const m = lines[0]?.match(/^(?:student|name|姓名)\s*[:：]\s*(.+)$/i);
      if (m) { name = m[1].trim(); body = lines.slice(1).join('\n').trim(); }
      return { index: i + 1, student_name: name, student_code: '', page_range: `Manual #${i + 1}`, raw_text: body, word_count: body.split(/\s+/).filter(Boolean).length };
    }));
  };

  const confirm = async () => {
    let final = segs;
    if (final.length === 0 && paste.trim()) {
      final = paste.split(/\n\s*---+\s*\n/).map((b, i) => ({
        index: i + 1,
        student_name: `Student #${String(i + 1).padStart(2, '0')}`,
        student_code: '', page_range: `Manual #${i + 1}`,
        raw_text: b.trim(), word_count: b.trim().split(/\s+/).filter(Boolean).length,
      })).filter(s => s.raw_text);
    }
    if (final.length === 0) { setErr('没有可导入的答卷'); return; }
    setBusy(true); setErr('');
    try {
      const res = await api.importSubmissions(psetId, final, autoGrade);
      const warning = res.task_error ? `答卷已导入，但批改任务创建失败：${res.task_error}` : undefined;
      onDone(res.task, warning);
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : '导入失败'); }
    finally { setBusy(false); }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/30 backdrop-blur-[2px] flex items-start justify-center pt-10 p-4 overflow-y-auto">
      <div className="w-full max-w-2xl bg-white rounded-2xl border border-[#E2DFD7] shadow-xl mb-12 flex flex-col max-h-[85vh]">
        <div className="flex items-center justify-between px-6 py-4 border-b border-[#EAE7DF]">
          <div>
            <div className="text-sm font-semibold text-[#191918]">导入答卷 · {psetTitle}</div>
            <div className="text-xs text-[#78756E] mt-0.5">上传批量 PDF 或直接粘贴文本，导入前可调整切分</div>
          </div>
          <button onClick={onClose} disabled={busy} className="p-1.5 rounded-lg hover:bg-[#F2F0EA] text-[#78756E] disabled:opacity-40">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-4 text-xs">
          {err && <div className="bg-[#FFE4E6] text-[#9F1239] rounded-xl px-3 py-2">{err}</div>}

          <div className="flex flex-wrap items-center gap-3 p-3 rounded-xl bg-[#FAF9F6] border border-[#E5E2DA]">
            <label className="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl bg-[#191918] text-white font-medium hover:bg-[#2E2D2A] cursor-pointer">
              <Upload className="w-3.5 h-3.5" /> {file ? `重选 (${file.name})` : '选择 PDF'}
              <input type="file" accept=".pdf" className="hidden" onChange={e => onFile(e.target.files?.[0] || null)} />
            </label>
            <select
              value={mode}
              onChange={e => { const m = e.target.value as 'auto' | 'pages_per_student'; setMode(m); if (file) parsePdf(file, m, pps); }}
              className="rounded-lg border border-[#DCD9D0] bg-white px-2 py-1.5"
            >
              <option value="auto">智能识别学生页眉</option>
              <option value="pages_per_student">固定页数切分</option>
            </select>
            {mode === 'pages_per_student' && (
              <div className="flex items-center gap-1">
                <span>每</span>
                <input
                  type="number" min={1} max={10} value={pps}
                  onChange={e => { const n = Math.max(1, Number(e.target.value) || 1); setPps(n); if (file) parsePdf(file, mode, n); }}
                  className="w-10 rounded-lg border border-[#DCD9D0] bg-white px-2 py-1 text-center"
                />
                <span>页</span>
              </div>
            )}
            {pages > 0 && <span className="text-[#78756E]">{pages} 页 → {segs.length} 份</span>}
          </div>

          <div>
            <div className="text-[11px] text-[#78756E] mb-1">
              或粘贴文本（多位学生用 <code className="font-mono bg-[#F2F0EA] px-1 rounded">---</code> 分隔，首行可写 Student: 名字）
            </div>
            <textarea
              rows={3} value={paste} onChange={e => setPaste(e.target.value)}
              placeholder={'Student: Alice\nHer essay response...\n---\nStudent: Bob\nHis essay...'}
              className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] p-3 font-mono text-[11px] leading-relaxed"
            />
            {paste.trim() && (
              <button onClick={fromPaste} className="mt-1.5 text-[11px] text-[#57544E] hover:text-[#191918]">
                → 生成预览卡片
              </button>
            )}
          </div>

          {segs.length > 0 && (
            <div className="space-y-2">
              {segs.map((seg, i) => (
                <div key={i} className="rounded-xl bg-[#FAF9F6] border border-[#E5E2DA] p-3 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-[#A39F96]">#{i + 1}</span>
                    <input
                      value={seg.student_name}
                      onChange={e => { const v = e.target.value; setSegs(prev => prev.map((s, j) => j === i ? { ...s, student_name: v } : s)); }}
                      className="rounded-lg border border-[#E2DFD7] px-2 py-1 w-36 font-medium text-xs"
                    />
                    <span className="text-[10.5px] text-[#A39F96]">{seg.page_range} · {seg.word_count} 词</span>
                    <div className="flex-1" />
                    {i > 0 && <button onClick={() => mergePrev(i)} className="text-[10.5px] text-[#57544E] hover:text-[#191918]">合并至上</button>}
                    <button onClick={() => splitHalf(i)} className="text-[10.5px] text-[#57544E] hover:text-[#191918]">拆分</button>
                    <button onClick={() => removeSeg(i)} className="text-[#9F1239] hover:opacity-70"><X className="w-3.5 h-3.5" /></button>
                  </div>
                  <textarea
                    rows={2} value={seg.raw_text}
                    onChange={e => { const v = e.target.value; setSegs(prev => prev.map((s, j) => j === i ? { ...s, raw_text: v, word_count: v.split(/\s+/).filter(Boolean).length } : s)); }}
                    className="w-full rounded-lg border border-[#EFECE4] bg-white p-2 text-[11px] leading-relaxed"
                  />
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-6 py-3.5 border-t border-[#EAE7DF]">
          <label className="inline-flex items-center gap-1.5 text-[11px] text-[#3F3D39]">
            <input type="checkbox" checked={autoGrade} onChange={e => setAutoGrade(e.target.checked)} className="accent-[#191918]" />
            入库后立即 AI 批改
          </label>
          <div className="flex gap-2">
            <button onClick={onClose} disabled={busy} className="px-4 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE] disabled:opacity-40">取消</button>
            <button onClick={confirm} disabled={busy} className="px-5 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium disabled:opacity-50">
              {busy ? '处理中…' : `导入${segs.length ? ` (${segs.length})` : ''}`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

/* ══════════════════════════════════════════════════════════════════
   ShortcutsModal
   ══════════════════════════════════════════════════════════════════ */
const ShortcutsModal: React.FC<{ onClose: () => void }> = ({ onClose }) => {
  const rows: [string[], string][] = [
    [['J', '↓'],      '下一份'],
    [['K', '↑'],      '上一份'],
    [['1-6'],         '聚焦采分项'],
    [['Alt', '0/1/2'],'修改当前项分值'],
    [['↵'],           '核定并跳到下一份'],
    [['G'],           '重跑 AI 评阅'],
    [['?'],           '本面板'],
  ];
  return (
    <div
      className="fixed inset-0 z-50 bg-black/30 backdrop-blur-[2px] flex items-center justify-center p-4"
      onClick={onClose}
    >
      <div
        className="w-80 bg-white rounded-2xl border border-[#E2DFD7] shadow-xl"
        onClick={e => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-[#EAE7DF]">
          <div className="text-sm font-semibold text-[#191918]">快捷键</div>
          <button onClick={onClose} className="p-1 rounded-lg hover:bg-[#F2F0EA] text-[#78756E]">
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="p-5 space-y-2 text-xs">
          {rows.map(([keys, desc], i) => (
            <div key={i} className="flex justify-between items-center">
              <span className="text-[#4A4741]">{desc}</span>
              <div className="flex gap-1">
                {keys.map(k => (
                  <kbd key={k} className="px-2 py-0.5 rounded bg-[#F4F2EC] border border-[#DCD9D0] font-mono text-[11px]">{k}</kbd>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
