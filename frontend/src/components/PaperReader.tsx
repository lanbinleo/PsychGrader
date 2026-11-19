import React, { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import {
  AlertTriangle,
  BookOpenCheck,
  Brain,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Columns2,
  FileText,
  Languages,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import type { Annotation, AnnotationCategory, Submission } from '../types';

interface Props {
  submission: Submission;
  activePartId: string | null;
  onSelectPart: (partId: string) => void;
  onTriggerGrade: () => void;
  isGrading: boolean;
}

const CATEGORY_META: Record<
  AnnotationCategory,
  {
    label: string;
    bgClass: string;
    pillClass: string;
    dotColor: string;
  }
> = {
  claim: {
    label: 'Claim 论点',
    bgClass: 'bg-[#FEF3C7]/90 border-b-2 border-[#B45309]/65 text-[#191918] rounded-[2px] px-0.5',
    pillClass: 'bg-[#FEF3C7] text-[#92400E] border-[#FDE68A]',
    dotColor: '#B45309',
  },
  evidence: {
    label: 'Evidence 文献证据',
    bgClass: 'bg-[#E0F2FE]/85 border-b-2 border-[#0369A1]/55 text-[#191918] rounded-[2px] px-0.5',
    pillClass: 'bg-[#E0F2FE] text-[#075985] border-[#BAE6FD]',
    dotColor: '#0369A1',
  },
  explanation: {
    label: 'Reasoning 推理桥',
    bgClass: 'bg-[#DCFCE7]/80 border-b-2 border-[#15803D]/55 text-[#191918] rounded-[2px] px-0.5',
    pillClass: 'bg-[#DCFCE7] text-[#166534] border-[#BBF7D0]',
    dotColor: '#15803D',
  },
  concept: {
    label: 'AP Concept 概念运用',
    bgClass: 'bg-[#FFEDD5]/95 border-b-2 border-[#C2410C]/70 text-[#191918] rounded-[2px] px-0.5 font-medium',
    pillClass: 'bg-[#FFEDD5] text-[#9A3412] border-[#FED7AA]',
    dotColor: '#C2410C',
  },
  issue: {
    label: '失分/矛盾警示',
    bgClass:
      'bg-[#FFE4E6]/85 underline decoration-wavy decoration-[#BE123C] decoration-[1.5px] underline-offset-4 text-[#191918] rounded-[2px] px-0.5',
    pillClass: 'bg-[#FFE4E6] text-[#9F1239] border-[#FECDD3]',
    dotColor: '#BE123C',
  },
};

export const PaperReader: React.FC<Props> = ({
  submission,
  activePartId,
  onSelectPart,
  onTriggerGrade,
  isGrading,
}) => {
  const [hoveredSentenceId, setHoveredSentenceId] = useState<string | null>(null);
  const [hoveredAnnotation, setHoveredAnnotation] = useState<Annotation | null>(null);
  const [viewMode, setViewMode] = useState<'bilingual' | 'en_only' | 'zh_only'>('bilingual');
  const [hiddenCategories, setHiddenCategories] = useState<Record<string, boolean>>({});
  const [showThinking, setShowThinking] = useState(false);

  const result = submission.result;

  const toggleCategory = (cat: AnnotationCategory) => {
    setHiddenCategories((prev) => ({ ...prev, [cat]: !prev[cat] }));
  };

  // Render an English sentence with exact substring highlights
  const renderHighlightedSentence = (sentId: string, text: string) => {
    if (!result) return text;
    const anns = result.annotations.filter(
      (a) => a.sentence_id === sentId && !hiddenCategories[a.category]
    );
    if (anns.length === 0) return text;

    // Locate non-overlapping intervals in text
    type Span = { start: number; end: number; ann: Annotation };
    const spans: Span[] = [];
    const lowerText = text.toLowerCase();

    for (const ann of anns) {
      const q = (ann.exact_quote || '').trim();
      let start = -1;
      let end = -1;
      if (q) {
        const idx = lowerText.indexOf(q.toLowerCase());
        if (idx !== -1) {
          start = idx;
          end = idx + q.length;
        }
      }
      // Fallback: if LLM slightly truncated quote, highlight whole sentence if no other span yet
      if (start === -1 && spans.length === 0) {
        start = 0;
        end = text.length;
      }
      if (start !== -1 && end > start) {
        const overlaps = spans.some((s) => !(end <= s.start || start >= s.end));
        if (!overlaps) {
          spans.push({ start, end, ann });
        }
      }
    }

    if (spans.length === 0) return text;
    spans.sort((a, b) => a.start - b.start);

    const nodes: React.ReactNode[] = [];
    let cursor = 0;
    spans.forEach((sp, idx) => {
      if (sp.start > cursor) {
        nodes.push(<span key={`plain-${idx}`}>{text.slice(cursor, sp.start)}</span>);
      }
      const meta = CATEGORY_META[sp.ann.category] || CATEGORY_META.explanation;
      const isPartFocused = activePartId && sp.ann.part_id === activePartId;
      nodes.push(
        <span
          key={`hl-${sp.ann.id}-${idx}`}
          onClick={(e) => {
            e.stopPropagation();
            onSelectPart(sp.ann.part_id);
            setHoveredAnnotation((cur) => (cur?.id === sp.ann.id ? null : sp.ann));
          }}
          onMouseEnter={() => setHoveredAnnotation(sp.ann)}
          className={`cursor-pointer transition-all duration-150 ${meta.bgClass} ${
            isPartFocused ? 'ring-2 ring-[#191918]/30 shadow-xs' : ''
          }`}
        >
          {text.slice(sp.start, sp.end)}
          <sup className="ml-1 inline-flex items-center rounded px-1 py-0 text-[10px] font-sans font-semibold tracking-tight bg-white/90 text-[#3F3D39] border border-[#DFDCD4] align-baseline">
            {sp.ann.badge}
          </sup>
        </span>
      );
      cursor = sp.end;
    });
    if (cursor < text.length) {
      nodes.push(<span key="plain-tail">{text.slice(cursor)}</span>);
    }
    return nodes;
  };

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Paper Top Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-2 pb-3">
        <div className="flex items-center gap-2.5">
          <div className="flex items-baseline gap-2">
            <h2 className="text-[16px] font-semibold tracking-tight text-[#191918]">
              {submission.student_name.trim()}
            </h2>
            {submission.student_code && (
              <span className="text-xs font-mono text-[#78756E]">{submission.student_code}</span>
            )}
            {submission.page_range && (
              <span className="text-[11px] px-1.5 py-0.5 rounded bg-[#EAE7DF] text-[#57544E] font-mono">
                {submission.page_range}
              </span>
            )}
          </div>
          <span className="text-xs text-[#8C8880]">·</span>
          <span className="text-xs text-[#6E6B65] tabular-nums">
            {submission.raw_text.trim().split(/\s+/).filter(Boolean).length} 词
          </span>
        </div>

        <div className="flex items-center gap-2">
          {/* View mode toggle */}
          <div className="inline-flex items-center rounded-lg bg-[#EAE8E2] p-0.5 text-xs">
            <button
              onClick={() => setViewMode('bilingual')}
              className={`flex items-center gap-1 px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'bilingual'
                  ? 'bg-white text-[#191918] shadow-2xs font-medium'
                  : 'text-[#6E6B65] hover:text-[#191918]'
              }`}
            >
              <Columns2 className="w-3.5 h-3.5" />
              双语对照
            </button>
            <button
              onClick={() => setViewMode('en_only')}
              className={`flex items-center gap-1 px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'en_only'
                  ? 'bg-white text-[#191918] shadow-2xs font-medium'
                  : 'text-[#6E6B65] hover:text-[#191918]'
              }`}
            >
              <FileText className="w-3.5 h-3.5" />
              仅原文
            </button>
            <button
              onClick={() => setViewMode('zh_only')}
              className={`flex items-center gap-1 px-2.5 py-1 rounded-md transition-colors ${
                viewMode === 'zh_only'
                  ? 'bg-white text-[#191918] shadow-2xs font-medium'
                  : 'text-[#6E6B65] hover:text-[#191918]'
              }`}
            >
              <Languages className="w-3.5 h-3.5" />
              仅直译
            </button>
          </div>

          <button
            onClick={onTriggerGrade}
            disabled={isGrading || submission.status === 'grading'}
            title="快捷键 G：使用当前模型重新评阅本份答卷"
            className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-[#DCD9D0] bg-white hover:bg-[#FAF9F6] text-xs font-medium text-[#2E2D2A] transition disabled:opacity-50 cursor-pointer"
          >
            <RefreshCw
              className={`w-3.5 h-3.5 ${
                isGrading || submission.status === 'grading' ? 'animate-spin' : ''
              }`}
            />
            {isGrading || submission.status === 'grading' ? '评阅中…' : '重跑 AI (G)'}
          </button>
        </div>
      </div>

      {/* Interactive Highlighter Filter Legend */}
      {result && (
        <div className="flex flex-wrap items-center justify-between gap-2 mb-3 px-3 py-2 rounded-xl bg-white border border-[#E6E3DC]">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[11px] text-[#8C8880] mr-1">高亮图层：</span>
            {(Object.keys(CATEGORY_META) as AnnotationCategory[]).map((cat) => {
              const meta = CATEGORY_META[cat];
              const count = result.annotations.filter((a) => a.category === cat).length;
              const off = !!hiddenCategories[cat];
              return (
                <button
                  key={cat}
                  onClick={() => toggleCategory(cat)}
                  className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] border transition cursor-pointer ${
                    off
                      ? 'bg-[#F6F5F1] text-[#A39F96] border-[#E6E3DC] line-through'
                      : meta.pillClass
                  }`}
                >
                  <span
                    className="w-1.5 h-1.5 rounded-full"
                    style={{ backgroundColor: off ? '#C5C1B8' : meta.dotColor }}
                  />
                  {meta.label}
                  <span className="opacity-70 tabular-nums">{count}</span>
                </button>
              );
            })}
          </div>

          {result.grader_mode === 'heuristic_fallback' && (
            <span className="text-[11px] text-[#92400E] bg-[#FEF3C7]/70 px-2 py-0.5 rounded">
              本地规则预检 · 填入 DeepSeek Key 可开启 V4.1 深度评阅
            </span>
          )}
        </div>
      )}

      {/* Main Archival Exam Paper Sheet */}
      <div className="flex-1 min-h-0 overflow-y-auto rounded-2xl bg-white border border-[#E5E2DA] shadow-[0_1px_3px_rgba(25,25,24,0.03)]">
        {/* AI Quick Summary Banner */}
        {result?.summary_zh && (
          <div className="px-6 py-3.5 bg-[#FAF9F5] border-b border-[#ECE9E1] flex items-start gap-2.5">
            <Sparkles className="w-4 h-4 text-[#6B5B3E] mt-0.5 shrink-0" />
            <div className="flex-1 text-[13px] leading-relaxed text-[#383632]">
              <span className="font-semibold text-[#191918] mr-1.5">阅卷速览：</span>
              {result.summary_zh}
            </div>
          </div>
        )}

        {/* Active Annotation / Concept Inspector Strip */}
        <AnimatePresence>
          {hoveredAnnotation && (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              exit={{ opacity: 0, height: 0 }}
              className="border-b border-[#ECE9E1] bg-[#F7F6F2] px-6 py-3 overflow-hidden"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-1 text-xs">
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      className={`px-2 py-0.5 rounded border font-medium ${
                        CATEGORY_META[hoveredAnnotation.category]?.pillClass
                      }`}
                    >
                      {hoveredAnnotation.badge}
                    </span>
                    <span className="text-[#2B2A27] font-medium">
                      {hoveredAnnotation.comment_zh}
                    </span>
                  </div>

                  {hoveredAnnotation.concept_check && (
                    <div className="flex flex-wrap items-center gap-2 pt-1 text-[11px] text-[#57544E]">
                      <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-white border border-[#E2DFD7] font-medium text-[#191918]">
                        <Brain className="w-3 h-3 text-[#C2410C]" />
                        {hoveredAnnotation.concept_check.canonical_term ||
                          hoveredAnnotation.concept_check.term}
                      </span>
                      {hoveredAnnotation.concept_check.ced_unit && (
                        <span className="px-1.5 py-0.5 rounded bg-[#EAE7DF] text-[#4A4741]">
                          {hoveredAnnotation.concept_check.ced_unit}
                        </span>
                      )}
                      {hoveredAnnotation.concept_check.in_ap_ced &&
                      !hoveredAnnotation.concept_check.is_ineligible ? (
                        <span className="inline-flex items-center gap-1 text-[#15803D] font-medium">
                          <CheckCircle2 className="w-3 h-3" /> 考纲有效概念
                        </span>
                      ) : null}
                      {hoveredAnnotation.concept_check.is_ineligible && (
                        <span className="inline-flex items-center gap-1 text-[#BE123C] font-semibold">
                          <AlertTriangle className="w-3 h-3" /> 命中本题禁用词黑名单
                        </span>
                      )}
                      {hoveredAnnotation.concept_check.duplicate_with_part_b && (
                        <span className="inline-flex items-center gap-1 text-[#BE123C] font-semibold">
                          <AlertTriangle className="w-3 h-3" /> 与 Part B 概念重复
                        </span>
                      )}
                      {!hoveredAnnotation.concept_check.applied_properly &&
                        !hoveredAnnotation.concept_check.is_ineligible && (
                          <span className="inline-flex items-center gap-1 text-[#B45309] font-medium">
                            <AlertTriangle className="w-3 h-3" /> 仅背定义或运用有误
                          </span>
                        )}
                      {hoveredAnnotation.concept_check.note_zh && (
                        <span className="text-[#6E6B65]">
                          — {hoveredAnnotation.concept_check.note_zh}
                        </span>
                      )}
                    </div>
                  )}
                </div>
                <button
                  onClick={() => setHoveredAnnotation(null)}
                  className="text-[11px] text-[#8C8880] hover:text-[#191918] shrink-0 cursor-pointer"
                >
                  收起
                </button>
              </div>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Body Content */}
        {!result ? (
          <div className="p-8 space-y-5">
            {submission.error_msg && (
              <div className="p-3.5 rounded-xl bg-[#FFE4E6]/60 border border-[#FECDD3] text-xs text-[#9F1239]">
                评阅出错：{submission.error_msg}
              </div>
            )}
            <div className="flex items-center justify-between">
              <span className="text-xs uppercase tracking-wider text-[#8C8880] font-medium">
                Student Raw Response
              </span>
              <button
                onClick={onTriggerGrade}
                disabled={isGrading || submission.status === 'grading'}
                className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-[#1F2924] text-white text-xs font-medium hover:bg-[#2E3C35] transition disabled:opacity-50 cursor-pointer"
              >
                <BookOpenCheck className="w-3.5 h-3.5" />
                {isGrading || submission.status === 'grading' ? '正在分析答卷…' : '立即运行 AI 评阅'}
              </button>
            </div>
            <div className="whitespace-pre-wrap text-[15px] leading-[1.85] text-[#22211F] font-editorial-serif">
              {submission.raw_text}
            </div>
          </div>
        ) : (
          <div className="divide-y divide-[#F0EDE6]">
            {/* Column Subheaders */}
            {viewMode === 'bilingual' && (
              <div className="grid grid-cols-12 bg-[#FBFAF7] text-[11px] font-medium text-[#858179] uppercase tracking-wider">
                <div className="col-span-7 px-6 py-2 border-r border-[#EFECE4]">
                  Student Original · 原文采分锚点 (58%)
                </div>
                <div className="col-span-5 px-5 py-2">
                  Literal Translation · 忠实直译对照 (42%)
                </div>
              </div>
            )}

            {/* Sentence-Aligned Rows */}
            <div className="divide-y divide-[#F4F2EC]">
              {result.sentences.map((sent) => {
                const isHovered = hoveredSentenceId === sent.id;
                const sentAnns = result.annotations.filter((a) => a.sentence_id === sent.id);
                const matchesActivePart =
                  activePartId && sentAnns.some((a) => a.part_id === activePartId);

                if (viewMode === 'en_only') {
                  return (
                    <div
                      key={sent.id}
                      onMouseEnter={() => setHoveredSentenceId(sent.id)}
                      onMouseLeave={() => setHoveredSentenceId(null)}
                      className={`px-7 py-3.5 transition-colors ${
                        isHovered || matchesActivePart ? 'bg-[#F8F6F0]' : ''
                      }`}
                    >
                      <span className="inline-block w-7 text-[11px] font-mono text-[#A39F96] select-none">
                        {sent.id}
                      </span>
                      <span className="text-[15px] leading-[1.9] text-[#1D1C1A]">
                        {renderHighlightedSentence(sent.id, sent.en)}
                      </span>
                    </div>
                  );
                }

                if (viewMode === 'zh_only') {
                  return (
                    <div
                      key={sent.id}
                      className="px-7 py-3.5 hover:bg-[#F8F6F0] transition-colors"
                    >
                      <span className="inline-block w-7 text-[11px] font-mono text-[#A39F96] select-none">
                        {sent.id}
                      </span>
                      <span className="text-[14.5px] leading-[1.85] text-[#2C2B28]">{sent.zh}</span>
                    </div>
                  );
                }

                return (
                  <div
                    key={sent.id}
                    onMouseEnter={() => setHoveredSentenceId(sent.id)}
                    onMouseLeave={() => setHoveredSentenceId(null)}
                    className={`grid grid-cols-12 transition-colors duration-150 ${
                      matchesActivePart
                        ? 'bg-[#F5F2E8]'
                        : isHovered
                        ? 'bg-[#FAF8F3]'
                        : 'bg-white'
                    }`}
                  >
                    <div className="col-span-7 px-6 py-3.5 border-r border-[#EFECE4] flex items-baseline gap-2.5">
                      <span className="text-[10.5px] font-mono text-[#ACA89F] shrink-0 select-none w-5">
                        {sent.id}
                      </span>
                      <div className="text-[14.5px] leading-[1.88] text-[#191918]">
                        {renderHighlightedSentence(sent.id, sent.en)}
                      </div>
                    </div>
                    <div className="col-span-5 px-5 py-3.5 flex items-baseline gap-2">
                      <div
                        className={`text-[13.5px] leading-[1.82] transition-colors ${
                          isHovered ? 'text-[#191918]' : 'text-[#4E4C47]'
                        }`}
                      >
                        {sent.zh}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Optional DeepSeek V4.1 Reasoning Trace */}
            {result.reasoning_trace && (
              <div className="px-6 py-3 bg-[#FAF9F6]">
                <button
                  onClick={() => setShowThinking((v) => !v)}
                  className="flex items-center gap-1.5 text-xs text-[#6E6B65] hover:text-[#191918] cursor-pointer"
                >
                  {showThinking ? (
                    <ChevronDown className="w-3.5 h-3.5" />
                  ) : (
                    <ChevronRight className="w-3.5 h-3.5" />
                  )}
                  查看模型内部阅卷推理链 ({result.model_used})
                </button>
                {showThinking && (
                  <pre className="mt-2.5 p-3.5 rounded-xl bg-white border border-[#E6E3DC] text-[11.5px] leading-relaxed text-[#57544E] whitespace-pre-wrap font-mono max-h-60 overflow-y-auto">
                    {result.reasoning_trace}
                  </pre>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
