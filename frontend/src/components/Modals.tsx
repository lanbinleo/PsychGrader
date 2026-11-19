import React, { useEffect, useState } from 'react';
import {
  BookOpen,
  CheckCircle2,
  KeyRound,
  Keyboard,
  Loader2,
  Lock,
  Plus,
  Search,
  Settings,
  Trash2,
  X,
} from 'lucide-react';
import { api, setAuthToken } from '../api';
import type { AppSettings, CEDEntry, ProblemSet, SourceDoc } from '../types';

/* ---------------------- 1. Problem Set Create/Edit Modal ---------------------- */

export const ProblemSetModal: React.FC<{
  initial?: ProblemSet | null;
  onClose: () => void;
  onSaved: (ps: ProblemSet) => void;
}> = ({ initial, onClose, onSaved }) => {
  const [title, setTitle] = useState(initial?.title || '');
  const [qType, setQType] = useState<'EBQ' | 'AAQ'>(initial?.question_type || 'EBQ');
  const [promptText, setPromptText] = useState(
    initial?.prompt_text ||
      'Using the provided sources, propose a defensible claim about the psychological phenomenon described...'
  );
  const [sources, setSources] = useState<SourceDoc[]>(
    initial?.sources?.length
      ? initial.sources
      : [
          { id: 'S1', label: 'Source 1', title: '', summary: '' },
          { id: 'S2', label: 'Source 2', title: '', summary: '' },
          { id: 'S3', label: 'Source 3', title: '', summary: '' },
        ]
  );
  const [ineligibleRaw, setIneligibleRaw] = useState(
    (
      initial?.ineligible_concepts || [
        'distractor',
        'mean',
        'random assignment',
        'rehearsal',
        'retrieval',
        'standard deviation',
        'statistical significance',
      ]
    ).join(', ')
  );
  const [scoringNotes, setScoringNotes] = useState(initial?.scoring_notes || '');
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');

  const handleSave = async () => {
    if (!title.trim()) {
      setErr('请填写题集名称（例如：G11 期中模拟 EBQ — 间隔学习效应）');
      return;
    }
    setSaving(true);
    setErr('');
    try {
      const ineligible = ineligibleRaw
        .split(/[,，\n]/)
        .map((s) => s.trim())
        .filter(Boolean);
      const payload: Partial<ProblemSet> = {
        title: title.trim(),
        question_type: qType,
        prompt_text: promptText.trim(),
        sources: qType === 'EBQ' ? sources : sources.slice(0, 1),
        ineligible_concepts: ineligible,
        scoring_notes: scoringNotes.trim(),
      };
      const saved = initial
        ? await api.updateProblemSet(initial.id, payload)
        : await api.createProblemSet(payload);
      onSaved(saved);
      onClose();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 backdrop-blur-[2px] p-4">
      <div className="w-full max-w-3xl max-h-[88vh] flex flex-col rounded-2xl bg-[#FAF9F6] border border-[#DFDCD3] shadow-xl overflow-hidden">
        <div className="px-6 py-4 bg-white border-b border-[#EAE7DF] flex items-center justify-between">
          <div>
            <h3 className="text-base font-semibold text-[#191918]">
              {initial ? '编辑题集与材料配置' : '新建 AP Psychology 题集 (Problem Set)'}
            </h3>
            <p className="text-xs text-[#78756E] mt-0.5">
              录入题干、Source 摘要与禁用词清单后，AI 将严格对照本题证据链批改
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-[#78756E] hover:bg-[#F2F0EA] cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto p-6 space-y-4 text-xs">
          {err && (
            <div className="p-3 rounded-xl bg-[#FFE4E6] text-[#9F1239] border border-[#FECDD3]">
              {err}
            </div>
          )}

          <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
            <div className="md:col-span-3">
              <label className="block font-medium text-[#3F3D39] mb-1">题集名称</label>
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="例如：Unit 2 Quiz EBQ — Testing Effect & Memory"
                className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2 text-xs focus:outline-none focus:border-[#191918]"
              />
            </div>
            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">题型</label>
              <select
                value={qType}
                onChange={(e) => setQType(e.target.value as 'EBQ' | 'AAQ')}
                className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2 text-xs"
              >
                <option value="EBQ">EBQ（7 分 · 三篇文献论证）</option>
                <option value="AAQ">AAQ（7 分 · 研究方法分析）</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">
              题干指令 (Prompt / Question Stem)
            </label>
            <textarea
              rows={3}
              value={promptText}
              onChange={(e) => setPromptText(e.target.value)}
              className="w-full rounded-xl border border-[#DCD9D0] bg-white p-3 leading-relaxed focus:outline-none focus:border-[#191918]"
            />
          </div>

          {qType === 'EBQ' && (
            <div className="space-y-2.5">
              <div className="flex items-center justify-between">
                <label className="font-medium text-[#3F3D39]">
                  三篇文献材料摘要 (Sources 1–3，粘贴原文或核心实验结论均可)
                </label>
                <button
                  type="button"
                  onClick={() =>
                    setSources((prev) => [
                      ...prev,
                      {
                        id: `S${prev.length + 1}`,
                        label: `Source ${prev.length + 1}`,
                        title: '',
                        summary: '',
                      },
                    ])
                  }
                  className="inline-flex items-center gap-1 text-[11px] text-[#57544E] hover:text-[#191918] cursor-pointer"
                >
                  <Plus className="w-3 h-3" /> 添加材料
                </button>
              </div>

              {sources.map((src, idx) => (
                <div
                  key={idx}
                  className="rounded-xl bg-white border border-[#E5E2DA] p-3.5 space-y-2"
                >
                  <div className="flex items-center gap-2">
                    <span className="px-2 py-0.5 rounded bg-[#EAE7DF] font-semibold text-[#191918]">
                      {src.label}
                    </span>
                    <input
                      value={src.title}
                      onChange={(e) => {
                        const v = e.target.value;
                        setSources((prev) =>
                          prev.map((s, i) => (i === idx ? { ...s, title: v } : s))
                        );
                      }}
                      placeholder="文献标题 / 作者年份（例如：Kornell (2009) Flashcard Spacing）"
                      className="flex-1 rounded-lg border border-[#E5E2DA] px-2.5 py-1"
                    />
                    {sources.length > 1 && (
                      <button
                        type="button"
                        onClick={() => setSources((prev) => prev.filter((_, i) => i !== idx))}
                        className="p-1 text-[#9F1239] hover:bg-[#FFE4E6]/60 rounded cursor-pointer"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                  <textarea
                    rows={3}
                    value={src.summary}
                    onChange={(e) => {
                      const v = e.target.value;
                      setSources((prev) =>
                        prev.map((s, i) => (i === idx ? { ...s, summary: v } : s))
                      );
                    }}
                    placeholder="粘贴该 Source 的实验设计、自变量/因变量与核心定量结果，供 AI 核查学生是否张冠李戴…"
                    className="w-full rounded-lg border border-[#EFECE4] bg-[#FAF9F6] p-2.5 leading-relaxed"
                  />
                </div>
              ))}

              <div>
                <label className="block font-medium text-[#3F3D39] mb-1">
                  题干与材料已明文给出的禁用术语 (Ineligible Concepts，逗号分隔，B(ii)/C(ii) 引用不得分)
                </label>
                <input
                  value={ineligibleRaw}
                  onChange={(e) => setIneligibleRaw(e.target.value)}
                  placeholder="distractor, mean, random assignment, rehearsal, retrieval..."
                  className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2 font-mono text-[11.5px]"
                />
              </div>
            </div>
          )}

          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">
              补充采分口径 / 教师特别说明（选填，直接注入评分 Prompt）
            </label>
            <textarea
              rows={2}
              value={scoringNotes}
              onChange={(e) => setScoringNotes(e.target.value)}
              placeholder="例如：允许接受 distributed practice 背后的 encoding variability 解释；B(i)/C(i) 必须写出哪一组表现更好才给分…"
              className="w-full rounded-xl border border-[#DCD9D0] bg-white p-3 leading-relaxed"
            />
          </div>
        </div>

        <div className="px-6 py-3.5 bg-white border-t border-[#EAE7DF] flex justify-end gap-2">
          <button
            onClick={onClose}
            className="px-3.5 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE] cursor-pointer"
          >
            取消
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2F2E2B] disabled:opacity-50 cursor-pointer"
          >
            {saving ? '保存中…' : '保存题集'}
          </button>
        </div>
      </div>
    </div>
  );
};

/* ---------------------- 2. DeepSeek Model Settings Modal ---------------------- */

export const SettingsModal: React.FC<{
  settings: AppSettings | null;
  onClose: () => void;
  onUpdated: (s: AppSettings) => void;
}> = ({ settings, onClose, onUpdated }) => {
  const [baseUrl, setBaseUrl] = useState(settings?.api_base_url || 'https://api.deepseek.com');
  const [apiKey, setApiKey] = useState('');
  const [model, setModel] = useState(settings?.model || 'deepseek-flash');
  const [thinking, setThinking] = useState(settings?.thinking_enabled ?? true);
  const [effort, setEffort] = useState(settings?.reasoning_effort || 'medium');
  const [concurrency, setConcurrency] = useState(settings?.concurrency || 4);
  const [testing, setTesting] = useState(false);
  const [testMsg, setTestMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [saving, setSaving] = useState(false);

  const handleTest = async () => {
    setTesting(true);
    setTestMsg(null);
    try {
      const res = await api.testLLM({
        api_base_url: baseUrl,
        api_key: apiKey || undefined,
        model,
      });
      setTestMsg({
        ok: true,
        text: `连通成功 (${res.model})：${res.reply}`,
      });
    } catch (e: unknown) {
      setTestMsg({
        ok: false,
        text: e instanceof Error ? e.message : '连接测试失败',
      });
    } finally {
      setTesting(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const payload: Record<string, unknown> = {
        api_base_url: baseUrl.trim(),
        model: model.trim(),
        thinking_enabled: thinking,
        reasoning_effort: effort,
        concurrency,
      };
      if (apiKey.trim()) {
        payload.api_key = apiKey.trim();
      }
      const next = await api.updateSettings(payload);
      onUpdated(next);
      onClose();
    } catch (e: unknown) {
      setTestMsg({
        ok: false,
        text: e instanceof Error ? e.message : '保存设置失败',
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 backdrop-blur-[2px] p-4">
      <div className="w-full max-w-lg rounded-2xl bg-[#FAF9F6] border border-[#DFDCD3] shadow-xl overflow-hidden">
        <div className="px-6 py-4 bg-white border-b border-[#EAE7DF] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Settings className="w-4 h-4 text-[#191918]" />
            <h3 className="text-base font-semibold text-[#191918]">模型与引擎配置</h3>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg text-[#78756E] hover:bg-[#F2F0EA] cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-6 space-y-4 text-xs">
          <div>
            <label className="block font-medium text-[#3F3D39] mb-1">
              API Base URL（OpenAI 兼容端点）
            </label>
            <input
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2 font-mono"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="font-medium text-[#3F3D39]">DeepSeek API Key</label>
              {settings?.has_api_key ? (
                <span className="text-[11px] text-[#166534]">
                  已配置 ({settings.api_key_masked})
                </span>
              ) : (
                <span className="text-[11px] text-[#92400E]">
                  未配置（当前使用本地启发式校准引擎兜底）
                </span>
              )}
            </div>
            <div className="relative">
              <KeyRound className="w-3.5 h-3.5 text-[#8C8880] absolute left-3 top-2.5" />
              <input
                type="password"
                value={apiKey}
                onChange={(e) => setApiKey(e.target.value)}
                placeholder={
                  settings?.has_api_key ? '留空保持原 Key 不变，输入新值即覆盖' : 'sk-...'
                }
                className="w-full rounded-xl border border-[#DCD9D0] bg-white pl-8 pr-3 py-2 font-mono"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">评分模型</label>
              <select
                value={model}
                onChange={(e) => setModel(e.target.value)}
                className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2"
              >
                <option value="deepseek-flash">deepseek-flash (V4.1 Flash · 推荐极速)</option>
                <option value="deepseek-v4-pro">deepseek-v4-pro (V4 Pro · 深度复核)</option>
              </select>
            </div>

            <div>
              <label className="block font-medium text-[#3F3D39] mb-1">批量并发数</label>
              <input
                type="number"
                min={1}
                max={12}
                value={concurrency}
                onChange={(e) => setConcurrency(Math.max(1, Number(e.target.value) || 4))}
                className="w-full rounded-xl border border-[#DCD9D0] bg-white px-3 py-2"
              />
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-white border border-[#E5E2DA] flex items-center justify-between gap-3">
            <div>
              <div className="font-medium text-[#191918]">开启 Thinking 推理核查链</div>
              <p className="text-[11px] text-[#78756E] mt-0.5">
                先逐步验算跨段 Source 冲突与禁用词再输出 JSON，准确率显著更高
              </p>
            </div>
            <div className="flex items-center gap-2">
              {thinking && (
                <select
                  value={effort}
                  onChange={(e) => setEffort(e.target.value)}
                  className="rounded-lg border border-[#DCD9D0] bg-[#FAF9F6] px-2 py-1 text-[11px]"
                >
                  <option value="low">Low</option>
                  <option value="medium">Medium</option>
                  <option value="high">High</option>
                </select>
              )}
              <input
                type="checkbox"
                checked={thinking}
                onChange={(e) => setThinking(e.target.checked)}
                className="w-4 h-4 accent-[#191918] cursor-pointer"
              />
            </div>
          </div>

          {testMsg && (
            <div
              className={`p-3 rounded-xl border text-xs flex items-center gap-2 ${
                testMsg.ok
                  ? 'bg-[#DCFCE7]/70 border-[#BBF7D0] text-[#166534]'
                  : 'bg-[#FFE4E6] border-[#FECDD3] text-[#9F1239]'
              }`}
            >
              {testMsg.ok && <CheckCircle2 className="w-4 h-4 shrink-0" />}
              <span>{testMsg.text}</span>
            </div>
          )}
        </div>

        <div className="px-6 py-3.5 bg-white border-t border-[#EAE7DF] flex items-center justify-between">
          <button
            onClick={handleTest}
            disabled={testing}
            className="inline-flex items-center gap-1.5 px-3 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#3F3D39] hover:bg-[#F5F3EE] cursor-pointer"
          >
            {testing && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            测试 API 连通性
          </button>

          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-3.5 py-2 rounded-xl border border-[#DCD9D0] text-xs font-medium text-[#57544E] hover:bg-[#F5F3EE] cursor-pointer"
            >
              关闭
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="px-4 py-2 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2F2E2B] cursor-pointer"
            >
              保存设置
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

/* ---------------------- 3. Keyboard Shortcuts Modal ---------------------- */

export const ShortcutsModal: React.FC<{ onClose: () => void }> = ({ onClose }) => {
  const rows = [
    { keys: ['J', '↓'], desc: '切换到下一份学生答卷' },
    { keys: ['K', '↑'], desc: '切换到上一份学生答卷' },
    { keys: ['1', '2', '3', '4', '5'], desc: '聚焦右侧第 1–5 项采分卡（同时联动高亮正文对应句）' },
    { keys: ['0', '1', '2'], desc: '按住 Alt + 数字，或直接点击分段胶囊修改当前采分项分值' },
    { keys: ['Enter'], desc: '核定当前试卷总分并自动跳到下一份待复核答卷' },
    { keys: ['G'], desc: '对当前答卷立即运行 / 重跑 AI 评阅与直译' },
    { keys: ['I'], desc: '打开 PDF 批量拆卷与答卷导入工作台' },
    { keys: ['D'], desc: '打开 AP Psychology CED (2024–2026) 考纲术语速查词典' },
    { keys: ['?'], desc: '随时呼出或关闭本快捷键面板' },
  ];

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 backdrop-blur-[2px] p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-2xl bg-white border border-[#DFDCD3] shadow-xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-5 py-4 border-b border-[#EAE7DF] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Keyboard className="w-4 h-4 text-[#191918]" />
            <h3 className="text-sm font-semibold text-[#191918]">阅卷台键盘流快捷键</h3>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-[#78756E] hover:bg-[#F2F0EA] cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="p-5 space-y-2.5 text-xs">
          {rows.map((r, i) => (
            <div key={i} className="flex items-center justify-between gap-4 py-1">
              <span className="text-[#4A4741]">{r.desc}</span>
              <div className="flex items-center gap-1 shrink-0">
                {r.keys.map((k) => (
                  <kbd
                    key={k}
                    className="px-2 py-0.5 rounded-md bg-[#F4F2EC] border border-[#DCD9D0] font-mono text-[11px] text-[#191918]"
                  >
                    {k}
                  </kbd>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

/* ---------------------- 4. AP CED Glossary Quick Lookup Modal ---------------------- */

export const CEDGlossaryModal: React.FC<{ onClose: () => void }> = ({ onClose }) => {
  const [q, setQ] = useState('');
  const [entries, setEntries] = useState<CEDEntry[]>([]);

  useEffect(() => {
    const timer = setTimeout(() => {
      api
        .searchCED(q)
        .then(setEntries)
        .catch(() => {});
    }, 120);
    return () => clearTimeout(timer);
  }, [q]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/35 backdrop-blur-[2px] p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-2xl max-h-[80vh] flex flex-col rounded-2xl bg-[#FAF9F6] border border-[#DFDCD3] shadow-xl overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-5 py-4 bg-white border-b border-[#EAE7DF] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <BookOpen className="w-4 h-4 text-[#191918]" />
            <h3 className="text-sm font-semibold text-[#191918]">
              AP Psychology CED (2024–2026) 官方考纲概念核查库
            </h3>
          </div>
          <button
            onClick={onClose}
            className="p-1 rounded-lg text-[#78756E] hover:bg-[#F2F0EA] cursor-pointer"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-4 bg-white border-b border-[#EAE7DF]">
          <div className="relative">
            <Search className="w-4 h-4 text-[#8C8880] absolute left-3 top-2.5" />
            <input
              autoFocus
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="输入英文术语或单元检索（例如：spacing effect, encoding, Unit 2, Piaget…）"
              className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] pl-9 pr-3 py-2 text-xs focus:outline-none focus:border-[#191918]"
            />
          </div>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto p-4 space-y-2">
          {entries.map((item) => (
            <div
              key={item.term}
              className="rounded-xl bg-white border border-[#E5E2DA] p-3 text-xs"
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-semibold text-[#191918]">{item.term}</span>
                <span className="px-2 py-0.5 rounded-full bg-[#F2F0EA] text-[#57544E] text-[11px]">
                  {item.unit}
                </span>
              </div>
              <p className="text-[#57544E] mt-1 leading-relaxed">{item.definition}</p>
            </div>
          ))}
          {entries.length === 0 && (
            <div className="py-10 text-center text-xs text-[#8C8880]">
              未检索到匹配词条（注意：2024 新考纲已移除部分旧版纯历史流派词条）
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

/* ---------------------- 5. Single-Teacher Login Gate ---------------------- */

export const LoginScreen: React.FC<{ onLoggedIn: () => void }> = ({ onLoggedIn }) => {
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
      onLoggedIn();
    } catch (error: unknown) {
      setErr(error instanceof Error ? error.message : '口令不正确');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-[#F6F5F1] p-4">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-2xl bg-white border border-[#E2DFD7] p-7 shadow-sm space-y-5"
      >
        <div className="space-y-1">
          <div className="w-9 h-9 rounded-xl bg-[#191918] text-white flex items-center justify-center mb-3">
            <Lock className="w-4 h-4" />
          </div>
          <h1 className="text-lg font-semibold tracking-tight text-[#191918]">
            PsychGrader 教师阅卷台
          </h1>
          <p className="text-xs text-[#78756E]">
            AP Psychology EBQ / AAQ 证据链核查与双语批改系统
          </p>
        </div>

        {err && (
          <div className="p-2.5 rounded-xl bg-[#FFE4E6] text-[#9F1239] text-xs">{err}</div>
        )}

        <div>
          <label className="block text-xs font-medium text-[#3F3D39] mb-1.5">教师访问口令</label>
          <input
            type="password"
            autoFocus
            value={passcode}
            onChange={(e) => setPasscode(e.target.value)}
            placeholder="请输入访问口令…"
            className="w-full rounded-xl border border-[#DCD9D0] bg-[#FAF9F6] px-3.5 py-2.5 text-sm focus:outline-none focus:border-[#191918]"
          />
        </div>

        <button
          type="submit"
          disabled={loading || !passcode}
          className="w-full py-2.5 rounded-xl bg-[#191918] text-white text-xs font-medium hover:bg-[#2E2D2A] disabled:opacity-40 cursor-pointer"
        >
          {loading ? '验证中…' : '进入阅卷台'}
        </button>
      </form>
    </div>
  );
};
