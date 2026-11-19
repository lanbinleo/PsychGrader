import React from 'react';
import { motion } from 'framer-motion';
import {
  AlertCircle,
  Check,
  CheckCircle2,
  CornerDownLeft,
  RotateCcw,
  ShieldAlert,
} from 'lucide-react';
import type { RubricPartScore, Submission } from '../types';

interface Props {
  submission: Submission;
  parts: RubricPartScore[];
  teacherNotes: string;
  activePartId: string | null;
  onSelectPart: (partId: string) => void;
  onChangePartScore: (partId: string, score: number) => void;
  onResetToAI: () => void;
  onChangeNotes: (val: string) => void;
  onSaveAndNext: () => void;
  isSaving: boolean;
}

const FLAG_LABELS: Record<string, { text: string; tone: 'warn' | 'err' | 'ok' }> = {
  missing_citation: { text: '漏标 Source 出处', tone: 'err' },
  wrong_source: { text: 'Source 编号标错', tone: 'err' },
  duplicate_source: { text: '与 B 段同源冲突', tone: 'err' },
  duplicate_concept: { text: '与 B 段概念重复', tone: 'err' },
  ineligible_concept: { text: '误踩题干禁用词', tone: 'err' },
  definition_only: { text: '纯背定义未结合材料', tone: 'warn' },
  direction_reversed: { text: '实验结果方向说反', tone: 'err' },
  self_contradiction: { text: '前后自相矛盾熔断', tone: 'err' },
  oversimplified_claim: { text: '复述问句或主张空泛', tone: 'warn' },
  implicit_claim_rescued: { text: '隐含 Claim 救济生效', tone: 'ok' },
};

export const RubricPanel: React.FC<Props> = ({
  submission,
  parts,
  teacherNotes,
  activePartId,
  onSelectPart,
  onChangePartScore,
  onResetToAI,
  onChangeNotes,
  onSaveAndNext,
  isSaving,
}) => {
  const effectiveScore = parts.reduce(
    (sum, p) => sum + (p.teacher_score != null ? p.teacher_score : p.ai_score),
    0
  );
  const maxTotal = parts.reduce((sum, p) => sum + p.max_score, 0) || 7;
  const aiScore = submission.result?.ai_total_score ?? submission.ai_score ?? 0;
  const hasOverride = parts.some(
    (p) => p.teacher_score != null && p.teacher_score !== p.ai_score
  );

  return (
    <div className="w-[376px] shrink-0 flex flex-col h-full min-h-0 rounded-2xl bg-white border border-[#E5E2DA] shadow-[0_1px_3px_rgba(25,25,24,0.03)] overflow-hidden">
      {/* Sticky Score Summary Header */}
      <div className="px-5 py-4 bg-[#FAF9F5] border-b border-[#ECE9E1]">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <span className="text-xs font-medium text-[#78756E] uppercase tracking-wider">
                Official Rubric
              </span>
              {submission.status === 'reviewed' ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[#DCFCE7] text-[#166534]">
                  <CheckCircle2 className="w-3 h-3" /> 已复核
                </span>
              ) : submission.status === 'ai_graded' ? (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[#EAE7DF] text-[#4A4741]">
                  AI 预判就绪
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium bg-[#F3F1EC] text-[#8C8880]">
                  待评阅
                </span>
              )}
            </div>

            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-3xl font-semibold tracking-tight text-[#191918] tabular-nums">
                {parts.length > 0 ? effectiveScore : '—'}
              </span>
              <span className="text-sm text-[#8C8880] tabular-nums">/ {maxTotal} pts</span>
              {hasOverride && (
                <span className="ml-1 text-[11px] text-[#92400E] bg-[#FEF3C7] px-1.5 py-0.5 rounded tabular-nums">
                  AI 预估 {aiScore}
                </span>
              )}
            </div>
          </div>

          <div className="flex flex-col items-end gap-1.5">
            <button
              onClick={onSaveAndNext}
              disabled={isSaving || parts.length === 0}
              className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-xl bg-[#191918] hover:bg-[#2F2E2B] text-white text-xs font-medium transition shadow-xs disabled:opacity-40 cursor-pointer"
            >
              <Check className="w-3.5 h-3.5" />
              核定并下一份
              <kbd className="ml-0.5 px-1 py-0.2 rounded bg-white/15 text-[10px] font-mono">
                <CornerDownLeft className="w-2.5 h-2.5 inline" />
              </kbd>
            </button>
            {hasOverride && (
              <button
                onClick={onResetToAI}
                className="inline-flex items-center gap-1 text-[11px] text-[#78756E] hover:text-[#191918] cursor-pointer"
              >
                <RotateCcw className="w-3 h-3" /> 恢复 AI 建议分
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Scrollable Rubric Cards */}
      <div className="flex-1 min-h-0 overflow-y-auto p-3.5 space-y-2.5">
        {parts.length === 0 ? (
          <div className="py-14 text-center text-xs text-[#8C8880]">
            点击左侧「立即运行 AI 评阅」生成逐项采分卡
          </div>
        ) : (
          parts.map((part, idx) => {
            const curScore = part.teacher_score != null ? part.teacher_score : part.ai_score;
            const isSelected = activePartId === part.part_id;
            const scoreOptions = Array.from({ length: part.max_score + 1 }, (_, i) => i);

            return (
              <div
                key={part.part_id}
                onClick={() => onSelectPart(part.part_id)}
                className={`rounded-xl border p-3 transition-all cursor-pointer ${
                  isSelected
                    ? 'border-[#191918]/45 bg-[#FAF8F3] shadow-2xs'
                    : 'border-[#EAE7DF] bg-white hover:border-[#D5D1C7]'
                }`}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <kbd className="px-1.5 py-0.2 rounded bg-[#F2F0EA] border border-[#DFDCD3] text-[10px] font-mono text-[#6E6B65]">
                        {idx + 1}
                      </kbd>
                      <span className="text-[13px] font-semibold text-[#191918] truncate">
                        {part.label}
                      </span>
                    </div>
                    <p className="text-[11px] text-[#7E7B73] mt-0.5">{part.subtitle}</p>
                  </div>

                  {/* Tactile Segmented Score Selector */}
                  <div
                    className="inline-flex rounded-lg bg-[#F0EEE8] p-0.5 shrink-0"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {scoreOptions.map((val) => {
                      const active = curScore === val;
                      return (
                        <button
                          key={val}
                          onClick={() => onChangePartScore(part.part_id, val)}
                          className={`relative px-2.5 py-1 text-xs font-semibold rounded-md tabular-nums transition-colors cursor-pointer ${
                            active ? 'text-white' : 'text-[#6E6B65] hover:text-[#191918]'
                          }`}
                        >
                          {active && (
                            <motion.span
                              layoutId={`pill-${submission.id}-${part.part_id}`}
                              transition={{ type: 'spring', stiffness: 450, damping: 32 }}
                              className={`absolute inset-0 rounded-md ${
                                val === part.max_score
                                  ? 'bg-[#1F4E3B]'
                                  : val > 0
                                  ? 'bg-[#85581A]'
                                  : 'bg-[#64615A]'
                              }`}
                            />
                          )}
                          <span className="relative z-10">{val}</span>
                        </button>
                      );
                    })}
                  </div>
                </div>

                {/* Metadata Chips: Source / Concept / Rule Flags */}
                {(part.cited_source ||
                  part.applied_concept ||
                  (part.flags && part.flags.length > 0)) && (
                  <div className="flex flex-wrap gap-1.5 mt-2">
                    {part.cited_source && (
                      <span className="px-1.5 py-0.5 rounded text-[10.5px] font-medium bg-[#E0F2FE]/80 text-[#075985]">
                        {part.cited_source}
                      </span>
                    )}
                    {part.applied_concept && (
                      <span className="px-1.5 py-0.5 rounded text-[10.5px] font-medium bg-[#FFEDD5] text-[#9A3412]">
                        {part.applied_concept}
                      </span>
                    )}
                    {part.flags?.map((fl) => {
                      const meta = FLAG_LABELS[fl] || { text: fl, tone: 'warn' };
                      return (
                        <span
                          key={fl}
                          className={`inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10.5px] font-medium ${
                            meta.tone === 'err'
                              ? 'bg-[#FFE4E6] text-[#9F1239]'
                              : meta.tone === 'ok'
                              ? 'bg-[#DCFCE7] text-[#166534]'
                              : 'bg-[#FEF3C7] text-[#92400E]'
                          }`}
                        >
                          {meta.tone === 'err' ? (
                            <ShieldAlert className="w-3 h-3" />
                          ) : (
                            <AlertCircle className="w-3 h-3" />
                          )}
                          {meta.text}
                        </span>
                      );
                    })}
                  </div>
                )}

                {/* AI Justification */}
                {part.reason_zh && (
                  <p className="mt-2 text-[12px] leading-relaxed text-[#45433E]">
                    {part.reason_zh}
                  </p>
                )}
              </div>
            );
          })
        )}

        {/* Teacher Feedback Note */}
        <div className="pt-1">
          <label className="block text-[11px] font-medium text-[#78756E] mb-1">
            教师批注 / 学生反馈（随成绩单导出）
          </label>
          <textarea
            rows={2}
            value={teacherNotes}
            onChange={(e) => onChangeNotes(e.target.value)}
            placeholder="选填：给学生的简短评语或人工复核备注…"
            className="w-full rounded-xl border border-[#E2DFD7] bg-[#FAF9F6] px-3 py-2 text-xs text-[#191918] placeholder:text-[#A39F96] focus:outline-none focus:border-[#191918]"
          />
        </div>
      </div>
    </div>
  );
};
