import React, { useState } from 'react';
import {
  FileUp,
  Merge,
  Plus,
  Scissors,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import { api } from '../api';
import type { SplitSegment } from '../types';

interface Props {
  psetId: number;
  psetTitle: string;
  onClose: () => void;
  onImported: () => void;
}

export const ImportModal: React.FC<Props> = ({
  psetId,
  psetTitle,
  onClose,
  onImported,
}) => {
  const [tab, setTab] = useState<'pdf' | 'paste'>('pdf');
  const [file, setFile] = useState<File | null>(null);
  const [splitMode, setSplitMode] = useState<'auto' | 'pages_per_student'>('auto');
  const [pagesPerStudent, setPagesPerStudent] = useState(1);
  const [segments, setSegments] = useState<SplitSegment[]>([]);
  const [pageCount, setPageCount] = useState(0);
  const [pasteText, setPasteText] = useState('');
  const [autoGrade, setAutoGrade] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const runParsePDF = async (
    targetFile: File | null = file,
    mode = splitMode,
    pps = pagesPerStudent
  ) => {
    if (!targetFile) return;
    setBusy(true);
    setError('');
    try {
      const res = await api.parsePDF(psetId, targetFile, mode, pps);
      setPageCount(res.page_count);
      setSegments(res.segments || []);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'PDF 解析失败');
    } finally {
      setBusy(false);
    }
  };

  const handleFileChange = (f: File | null) => {
    setFile(f);
    if (f) {
      runParsePDF(f, splitMode, pagesPerStudent);
    }
  };

  const mergeWithPrev = (idx: number) => {
    if (idx <= 0) return;
    setSegments((prev) => {
      const copy = [...prev];
      const upper = copy[idx - 1];
      const cur = copy[idx];
      const combinedText = `${upper.raw_text}\n\n${cur.raw_text}`.trim();
      copy[idx - 1] = {
        ...upper,
        page_range:
          upper.page_range && cur.page_range
            ? `${upper.page_range}, ${cur.page_range}`
            : upper.page_range || cur.page_range,
        raw_text: combinedText,
        word_count: combinedText.split(/\s+/).filter(Boolean).length,
      };
      copy.splice(idx, 1);
      return copy.map((s, i) => ({ ...s, index: i + 1 }));
    });
  };

  const splitInHalf = (idx: number) => {
    setSegments((prev) => {
      const copy = [...prev];
      const cur = copy[idx];
      const paras = cur.raw_text.split(/\n\s*\n/).filter((p) => p.trim());
      if (paras.length < 2) return prev;
      const mid = Math.ceil(paras.length / 2);
      const firstTxt = paras.slice(0, mid).join('\n\n');
      const secondTxt = paras.slice(mid).join('\n\n');
      const a: SplitSegment = {
        ...cur,
        raw_text: firstTxt,
        word_count: firstTxt.split(/\s+/).filter(Boolean).length,
      };
      const b: SplitSegment = {
        index: cur.index + 1,
        student_name: `${cur.student_name} (Part 2)`,
        student_code: '',
        page_range: cur.page_range,
        raw_text: secondTxt,
        word_count: secondTxt.split(/\s+/).filter(Boolean).length,
      };
      copy.splice(idx, 1, a, b);
      return copy.map((s, i) => ({ ...s, index: i + 1 }));
    });
  };

  const removeSegment = (idx: number) => {
    setSegments((prev) => prev.filter((_, i) => i !== idx).map((s, i) => ({ ...s, index: i + 1 })));
  };

  const parsePasteBlocks = () => {
    const blocks = pasteText
      .split(/\n\s*---+\s*\n/)
      .map((b) => b.trim())
      .filter(Boolean);
    const built: SplitSegment[] = blocks.map((blk, i) => {
      const lines = blk.split('\n');
      let name = `Student #${String(i + 1).padStart(2, '0')}`;
      let body = blk;
      const m = lines[0]?.match(/^(?:student|name|姓名)\s*[:：]\s*(.+)$/i);
      if (m) {
        name = m[1].trim();
        body = lines.slice(1).join('\n').trim();
      }
      return {
        index: i + 1,
        student_name: name,
        student_code: '',
        page_range: `Manual #${i + 1}`,
        raw_text: body,
        word_count: body.split(/\s+/).filter(Boolean).length,
      };
    });
    setSegments(built);
  };

  const handleConfirmImport = async () => {
    let finalSegs = segments;
    if (tab === 'paste' && finalSegs.length === 0 && pasteText.trim()) {
      const blocks = pasteText
        .split(/\n\s*---+\s*\n/)
        .map((b) => b.trim())
        .filter(Boolean);
      finalSegs = blocks.map((blk, i) => ({
        index: i + 1,
        student_name: `Student #${String(i + 1).padStart(2, '0')}`,
        student_code: '',
        page_range: `Manual #${i + 1}`,
        raw_text: blk,
        word_count: blk.split(/\s+/).filter(Boolean).length,
      }));
    }
    if (finalSegs.length === 0) {
      setError('没有可导入的学生答卷');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await api.importSubmissions(psetId, finalSegs, autoGrade);
      onImported();
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : '导入失败');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 backdrop-blur-[2px] p-4">
      <div className="w-full max-w-4xl max-h-[88vh] flex flex-col rounded-2xl bg-[#FAF9F6] border border-[#DFDCD3] shadow-xl overflow-hidden">
        {/* Header */}
        <div className="px-6 py-4 bg-white border-b border-[#EAE7DF] flex items-center justify-between">
          <div>
            <h3 className="text-base font-semibold text-[#191918]">
              批量导入学生答卷 · {psetTitle}
            </h3>
            <p className="text-xs text-[#78756E] mt-0.5">
              支持 AP Classroom 批量导出的合并 PDF（先预览切分结果再入库，可随时合并跨页答卷）
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-[#78756E] hover:bg-[#F2F0EA] cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Mode Tabs */}
        <div className="px-6 pt-3 bg-white border-b border-[#EAE7DF] flex items-center gap-4 text-xs font-medium">
          <button
            onClick={() => setTab('pdf')}
            className={`pb-2.5 border-b-2 transition cursor-pointer ${
              tab === 'pdf'
                ? 'border-[#191918] text-[#191918]'
                : 'border-transparent text-[#78756E] hover:text-[#191918]'
            }`}
          >
            PDF 批量拆卷工作台
          </button>
          <button
            onClick={() => setTab('paste')}
            className={`pb-2.5 border-b-2 transition cursor-pointer ${
              tab === 'paste'
                ? 'border-[#191918] text-[#191918]'
                : 'border-transparent text-[#78756E] hover:text-[#191918]'
            }`}
          >
            文本直接粘贴（支持 `---` 批量分隔）
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-4">
          {error && (
            <div className="p-3 rounded-xl bg-[#FFE4E6] border border-[#FECDD3] text-xs text-[#9F1239]">
              {error}
            </div>
          )}

          {tab === 'pdf' ? (
            <>
              <div className="flex flex-wrap items-center justify-between gap-3 p-4 rounded-xl bg-white border border-[#E5E2DA]">
                <label className="inline-flex items-center gap-2 px-3.5 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A] transition cursor-pointer">
                  <FileUp className="w-4 h-4" />
                  {file ? `重选 PDF（当前：${file.name}）` : '选择合并答卷 PDF'}
                  <input
                    type="file"
                    accept="application/pdf,.pdf"
                    className="hidden"
                    onChange={(e) => handleFileChange(e.target.files?.[0] || null)}
                  />
                </label>

                <div className="flex items-center gap-2 text-xs">
                  <span className="text-[#6E6B65]">切分策略：</span>
                  <select
                    value={splitMode}
                    onChange={(e) => {
                      const m = e.target.value as 'auto' | 'pages_per_student';
                      setSplitMode(m);
                      if (file) runParsePDF(file, m, pagesPerStudent);
                    }}
                    className="rounded-lg border border-[#DCD9D0] bg-[#FAF9F6] px-2.5 py-1.5 text-xs"
                  >
                    <option value="auto">智能识别页眉 (Student / AP ID / 分页)</option>
                    <option value="pages_per_student">固定页数切分</option>
                  </select>

                  {splitMode === 'pages_per_student' && (
                    <div className="flex items-center gap-1">
                      <span className="text-[#6E6B65]">每</span>
                      <input
                        type="number"
                        min={1}
                        max={10}
                        value={pagesPerStudent}
                        onChange={(e) => {
                          const n = Math.max(1, Number(e.target.value) || 1);
                          setPagesPerStudent(n);
                          if (file) runParsePDF(file, splitMode, n);
                        }}
                        className="w-12 rounded-lg border border-[#DCD9D0] bg-white px-2 py-1 text-center text-xs"
                      />
                      <span className="text-[#6E6B65]">页一位学生</span>
                    </div>
                  )}
                </div>
              </div>

              {pageCount > 0 && (
                <div className="text-xs text-[#6E6B65]">
                  共读取 <strong className="text-[#191918]">{pageCount}</strong> 页 PDF，拆分为{' '}
                  <strong className="text-[#191918]">{segments.length}</strong> 份学生答卷（可在下方修改姓名、合并跨页或删除封面页）：
                </div>
              )}
            </>
          ) : (
            <div className="space-y-2.5">
              <textarea
                rows={7}
                value={pasteText}
                onChange={(e) => setPasteText(e.target.value)}
                placeholder={`可直接粘贴一位或多位学生的英文回答。\n多位学生之间用一行 --- 分隔；首行可选写 Student: Alice Chen 自动命名。`}
                className="w-full rounded-xl border border-[#DCD9D0] bg-white p-3.5 text-xs leading-relaxed focus:outline-none focus:border-[#191918]"
              />
              <button
                onClick={parsePasteBlocks}
                className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-[#DCD9D0] bg-white text-xs font-medium hover:bg-[#F2F0EA] cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" /> 生成预览卡片
              </button>
            </div>
          )}

          {/* Segments Preview List */}
          {segments.length > 0 && (
            <div className="space-y-2.5">
              {segments.map((seg, idx) => (
                <div
                  key={idx}
                  className="rounded-xl bg-white border border-[#E5E2DA] p-3.5 space-y-2"
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-mono text-[#8C8880]">#{idx + 1}</span>
                      <input
                        value={seg.student_name}
                        onChange={(e) => {
                          const val = e.target.value;
                          setSegments((prev) =>
                            prev.map((item, i) =>
                              i === idx ? { ...item, student_name: val } : item
                            )
                          );
                        }}
                        className="rounded-lg border border-[#E2DFD7] px-2.5 py-1 text-xs font-medium text-[#191918] w-44"
                        placeholder="学生姓名"
                      />
                      <input
                        value={seg.student_code}
                        onChange={(e) => {
                          const val = e.target.value;
                          setSegments((prev) =>
                            prev.map((item, i) =>
                              i === idx ? { ...item, student_code: val } : item
                            )
                          );
                        }}
                        className="rounded-lg border border-[#E2DFD7] px-2.5 py-1 text-xs font-mono text-[#6E6B65] w-28"
                        placeholder="AP ID (选填)"
                      />
                      <span className="text-[11px] text-[#8C8880] font-mono">
                        {seg.page_range} · {seg.word_count} 词
                      </span>
                    </div>

                    <div className="flex items-center gap-1.5">
                      {idx > 0 && (
                        <button
                          onClick={() => mergeWithPrev(idx)}
                          title="把当前页合并到上一位学生名下"
                          className="inline-flex items-center gap-1 px-2 py-1 rounded-md border border-[#E2DFD7] text-[11px] text-[#4A4741] hover:bg-[#F5F3EE] cursor-pointer"
                        >
                          <Merge className="w-3 h-3" /> 合并至上一份
                        </button>
                      )}
                      <button
                        onClick={() => splitInHalf(idx)}
                        title="按段落拆成两份"
                        className="inline-flex items-center gap-1 px-2 py-1 rounded-md border border-[#E2DFD7] text-[11px] text-[#4A4741] hover:bg-[#F5F3EE] cursor-pointer"
                      >
                        <Scissors className="w-3 h-3" /> 拆分
                      </button>
                      <button
                        onClick={() => removeSegment(idx)}
                        className="p-1 rounded-md text-[#9F1239] hover:bg-[#FFE4E6]/60 cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  <textarea
                    rows={3}
                    value={seg.raw_text}
                    onChange={(e) => {
                      const val = e.target.value;
                      setSegments((prev) =>
                        prev.map((item, i) =>
                          i === idx
                            ? {
                                ...item,
                                raw_text: val,
                                word_count: val.split(/\s+/).filter(Boolean).length,
                              }
                            : item
                        )
                      );
                    }}
                    className="w-full rounded-lg border border-[#EFECE4] bg-[#FAF9F6] p-2.5 text-xs text-[#2E2D2A] leading-relaxed"
                  />
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-3.5 bg-white border-t border-[#EAE7DF] flex items-center justify-between">
          <label className="inline-flex items-center gap-2 text-xs text-[#3F3D39] cursor-pointer">
            <input
              type="checkbox"
              checked={autoGrade}
              onChange={(e) => setAutoGrade(e.target.checked)}
              className="rounded accent-[#191918]"
            />
            入库后立即在后台并发运行 AI 预判与逐句直译
          </label>

          <div className="flex items-center gap-2">
            <button
              onClick={onClose}
              className="px-3.5 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE] cursor-pointer"
            >
              取消
            </button>
            <button
              onClick={handleConfirmImport}
              disabled={busy}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2F2E2B] disabled:opacity-50 cursor-pointer"
            >
              <Upload className="w-3.5 h-3.5" />
              {busy ? '处理中…' : `确认导入${segments.length ? ` (${segments.length} 份)` : ''}`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
