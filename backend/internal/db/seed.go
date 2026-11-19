package db

import (
	"encoding/json"
	"psychgrader/internal/models"
)

func (s *Store) seedIfEmpty() error {
	var cnt int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM problem_sets`).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}

	ebq := &models.ProblemSet{
		Title:        "2026 CB Official Calibration · Spaced vs. Massed Learning (EBQ)",
		QuestionType: "EBQ",
		PromptText:   "Using the provided sources, propose a claim about what strengthens a person's ability to retrieve newly learned information, and support your claim using evidence from two different sources and psychological concepts from AP Psychology.",
		Sources: []models.SourceDoc{
			{
				ID:      "Source 1",
				Label:   "Source 1",
				Title:   "Moulton et al. — Microsurgical Suturing Practice Schedule",
				Summary: "Thirty-eight surgical residents with no prior microsurgical suturing experience were randomly assigned to two groups. Group 1 completed an 8-hour training workshop in a single day. Group 2 completed the same 8 hours of training distributed across four weekly 2-hour sessions. One month after training ended, blind raters scored participants on a standardized suturing task. Group 2 achieved a significantly higher mean performance score (M = 31.60, SD = 3.1) than Group 1 (M = 27.63, SD = 4.2, p < .05).",
			},
			{
				ID:      "Source 2",
				Label:   "Source 2",
				Title:   "Rawson & Kintsch — Science Text Reading Intervals",
				Summary: "Undergraduate students read four complex science articles (astronomy, biology, physics, geology). Group 1 read all four articles back-to-back in a single afternoon session and took an immediate comprehension test 10 minutes later, scoring higher on the immediate test (M = 82%) than Group 2 (M = 74%). However, when retested one week later without notes, Group 2—who had read the articles across two sessions separated by 48 hours—maintained stable test scores (M = 73%), whereas Group 1 dropped sharply (M = 54%).",
			},
			{
				ID:      "Source 3",
				Label:   "Source 3",
				Title:   "Kramer et al. — Condensed Active Retrieval Modules in High School Biology",
				Summary: "High school students who completed three short 20-minute focused self-quizzing modules spaced across two weeks (1 hour total) performed just as well on a standardized national biology exam (M = 78.4) as students who sat through 23 hours of passive lecture review without self-testing (M = 77.9, difference not statistically significant).",
			},
		},
		IneligibleConcepts: []string{
			"distractor", "mean", "random assignment", "rehearsal", "retrieval", "standard deviation", "statistical significance",
		},
		ScoringNotes: "2026 College Board Official EBQ Scoring Guidelines: C(i) must cite a different source from B(i); C(ii) must apply a different AP Psychology concept from B(ii). Ineligible terms from the prompt/sources cannot earn the 2nd application point.",
	}
	if err := s.CreateProblemSet(ebq); err != nil {
		return err
	}

	type seedPaper struct {
		name   string
		code   string
		pages  string
		text   string
		status string
		result models.GradingResult
	}

	papers := []seedPaper{
		{
			name:   " Chloe Lin",
			code:   "AP-204918",
			pages:  "P.1",
			status: "ai_graded",
			text: `Distributing study and practice sessions across time with active recall intervals substantially strengthens long-term retention of both procedural skills and academic concepts.

In Source 1, surgical residents who practiced suturing across four weekly two-hour sessions achieved a higher mean score one month later (31.60) than residents who crammed all eight hours into a single day (27.63) (Source 1). This supports my claim because spreading practice across weeks leverages the spacing effect and allows sleep-dependent memory consolidation between sessions, stabilizing procedural motor memory traces in the cerebellum and basal ganglia so skills do not decay after a month.

Furthermore, undergraduates who read four science articles across two sessions separated by 48 hours maintained stable recall a week later (73%), whereas students who read all four articles back-to-back dropped from 82% to 54% (Source 2). Reading four dense science topics without a break creates severe retroactive and proactive interference, as newly read physics passages disrupt earlier biology traces before encoding finishes; inserting a 48-hour gap reduces interference and keeps long-term recall intact.`,
			result: models.GradingResult{
				AITotalScore:  7,
				MaxTotalScore: 7,
				GraderMode:    "calibration_seed",
				ModelUsed:     "deepseek-flash",
				SummaryZh:     "满分标杆卷（7/7）：首句立论精准涵盖程序性与陈述性记忆；B段引 Source 1 结合 Spacing Effect / Memory Consolidation，C段换用 Source 2 结合 Proactive & Retroactive Interference，跨源跨概念完全合规。",
				Sentences: []models.SentencePair{
					{ID: "S1", En: "Distributing study and practice sessions across time with active recall intervals substantially strengthens long-term retention of both procedural skills and academic concepts.", Zh: "将学习与练习分散在不同时间段并穿插主动提取间隔，能显著增强程序性技能与学科概念的长期保持。"},
					{ID: "S2", En: "In Source 1, surgical residents who practiced suturing across four weekly two-hour sessions achieved a higher mean score one month later (31.60) than residents who crammed all eight hours into a single day (27.63) (Source 1).", Zh: "在 Source 1 中，每周练习两小时、持续四周的外科住院医，一个月后的平均得分（31.60）高于把八小时全挤在一天内完成的住院医（27.63）（Source 1）。"},
					{ID: "S3", En: "This supports my claim because spreading practice across weeks leverages the spacing effect and allows sleep-dependent memory consolidation between sessions, stabilizing procedural motor memory traces in the cerebellum and basal ganglia so skills do not decay after a month.", Zh: "这支撑了我的主张，因为跨周分散练习利用了间隔效应（spacing effect），并允许各次练习之间发生依赖睡眠的记忆巩固，从而稳定小脑与基底核中的程序性运动记忆痕迹，使技能在一个月后不致衰退。"},
					{ID: "S4", En: "Furthermore, undergraduates who read four science articles across two sessions separated by 48 hours maintained stable recall a week later (73%), whereas students who read all four articles back-to-back dropped from 82% to 54% (Source 2).", Zh: "此外，分两次、间隔 48 小时阅读四篇科学文章的本科生在一周后保持了稳定的回忆率（73%），而一口气连读四篇的学生成绩从 82% 骤降至 54%（Source 2）。"},
					{ID: "S5", En: "Reading four dense science topics without a break creates severe retroactive and proactive interference, as newly read physics passages disrupt earlier biology traces before encoding finishes; inserting a 48-hour gap reduces interference and keeps long-term recall intact.", Zh: "不间断地阅读四个密集的科学主题会造成严重的倒摄与前摄抑制，因为新读到的物理段落会在编码完成前干扰早先的生物记忆痕迹；插入 48 小时间隔减少了干扰，使长期回忆保持完整。"},
				},
				Annotations: []models.Annotation{
					{ID: "A1", SentenceID: "S1", ExactQuote: "Distributing study and practice sessions across time with active recall intervals substantially strengthens long-term retention", Category: "claim", PartID: "part_a", Badge: "Part A · Claim (+1)", CommentZh: "明确提出分散学习增强长期记忆保持的可验证主张"},
					{ID: "A2", SentenceID: "S2", ExactQuote: "surgical residents who practiced suturing across four weekly two-hour sessions achieved a higher mean score one month later (31.60) than residents who crammed all eight hours into a single day (27.63) (Source 1)", Category: "evidence", PartID: "part_b_i", Badge: "B(i) · Source 1 证据 (+1)", CommentZh: "准确引用组间差异方向并明确标注 (Source 1)"},
					{ID: "A3", SentenceID: "S3", ExactQuote: "leverages the spacing effect and allows sleep-dependent memory consolidation between sessions", Category: "concept", PartID: "part_b_ii", Badge: "B(ii) · Spacing Effect 运用 (+2)", CommentZh: "结合外科缝合跨周训练解释记忆巩固机制，非单纯背定义", ConceptCheck: &models.ConceptCheck{Term: "spacing effect", CanonicalTerm: "Spacing Effect (Distributed Practice)", CEDUnit: "Unit 2: Cognition", InAPCED: true, IsIneligible: false, AppliedProperly: true, NoteZh: "AP CED Unit 2 核心概念，运用充分"}},
					{ID: "A4", SentenceID: "S4", ExactQuote: "undergraduates who read four science articles across two sessions separated by 48 hours maintained stable recall a week later (73%), whereas students who read all four articles back-to-back dropped from 82% to 54% (Source 2)", Category: "evidence", PartID: "part_c_i", Badge: "C(i) · Source 2 证据 (+1)", CommentZh: "更换为另一篇文献 Source 2，准确对比一周后保持率差异"},
					{ID: "A5", SentenceID: "S5", ExactQuote: "creates severe retroactive and proactive interference, as newly read physics passages disrupt earlier biology traces", Category: "concept", PartID: "part_c_ii", Badge: "C(ii) · Interference 运用 (+2)", CommentZh: "换用与 B 段不同的干扰理论（倒摄/前摄抑制）解释连读四篇遗忘原因", ConceptCheck: &models.ConceptCheck{Term: "retroactive interference", CanonicalTerm: "Retroactive Interference", CEDUnit: "Unit 2: Cognition", InAPCED: true, IsIneligible: false, AppliedProperly: true, NoteZh: "与 B(ii) 概念不重复，正确解释后读物理对先读生物的干扰"}},
				},
				RubricParts: []models.RubricPartScore{
					{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确主张", MaxScore: 1, AIScore: 1, ReasonZh: "S1 明确提出分散练习结合主动提取能增强程序性与陈述性记忆保持。", EvidenceQuote: "Distributing study and practice sessions across time... strengthens long-term retention"},
					{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "具体研究发现 + 明确文献标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 1", ReasonZh: "准确引述 Source 1 四周分散组一个月后缝合评分高于单日集中组，出处清晰。", EvidenceQuote: "practiced suturing across four weekly two-hour sessions achieved a higher mean score... (Source 1)"},
					{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "解释支撑关系 + 运用心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 1", AppliedConcept: "Spacing Effect (Distributed Practice)", ReasonZh: "既解释了证据如何支撑长期保持（+1），又结合睡眠依赖的记忆巩固与间隔效应作了深度运用（+1）。"},
					{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "另一篇不同文献的具体发现 + 标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 2", ReasonZh: "独立选用 Source 2，准确描述间隔 48 小时阅读组一周后成绩稳定（73% vs 54%）。"},
					{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "解释支撑关系 + 第二个不同心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 2", AppliedConcept: "Retroactive Interference", ReasonZh: "准确解释连读四篇文章为何导致一周后暴跌（+1），并正确运用与 B 段不同的干扰理论（Retroactive Interference）（+1）。"},
				},
			},
		},
		{
			name:   "Ethan Zhang",
			code:   "AP-204925",
			pages:  "P.2",
			status: "ai_graded",
			text: `Breaking study time into spaced, shorter sessions improves how well students remember material over the long run.

According to Source 1, medical trainees who learned surgical suturing in four weekly two-hour sessions scored higher one month later than trainees who practiced for eight hours straight in one day. This proves my claim through the spacing effect, because spreading practice out gives the brain time to consolidate the skill into long-term memory instead of cramming it all at once.

In addition, students who read four science articles over two separate days kept a 73% score a week later while students who read all four in one afternoon dropped to 54%. This also supports my claim because of distributed practice: when students space their reading across days rather than cramming, the spacing effect helps them retain the science articles much better.`,
			result: models.GradingResult{
				AITotalScore:  5,
				MaxTotalScore: 7,
				GraderMode:    "calibration_seed",
				ModelUsed:     "deepseek-flash",
				SummaryZh:     "典型高频失分卷（5/7）：两处硬伤——① C(i) 引用了四周科学阅读数据却漏标 `(Source 2)` 出处（丢 1 分，但按官方解耦规则不影响 C(ii) 取分）；② C(ii) 把 Distributed Practice 当新概念，实则与 B(ii) 的 Spacing Effect 同义撞车，扣第 2 分概念分。",
				Sentences: []models.SentencePair{
					{ID: "S1", En: "Breaking study time into spaced, shorter sessions improves how well students remember material over the long run.", Zh: "把学习时间拆分成有间隔的较短时段，能提升学生长期记住材料的效果。"},
					{ID: "S2", En: "According to Source 1, medical trainees who learned surgical suturing in four weekly two-hour sessions scored higher one month later than trainees who practiced for eight hours straight in one day.", Zh: "根据 Source 1，分四周、每周两小时学习外科缝合的医学生，在一个月后的得分高于一天连续练习八小时的学生。"},
					{ID: "S3", En: "This proves my claim through the spacing effect, because spreading practice out gives the brain time to consolidate the skill into long-term memory instead of cramming it all at once.", Zh: "这通过间隔效应（spacing effect）证明了我的主张，因为把练习分散开给了大脑将技能巩固进长时记忆的时间，而不是一下子硬塞进去。"},
					{ID: "S4", En: "In addition, students who read four science articles over two separate days kept a 73% score a week later while students who read all four in one afternoon dropped to 54%.", Zh: "此外，分两天阅读四篇科学文章的学生在一周后保持了 73% 的分数，而在一个下午读完四篇的学生掉到了 54%。"},
					{ID: "S5", En: "This also supports my claim because of distributed practice: when students space their reading across days rather than cramming, the spacing effect helps them retain the science articles much better.", Zh: "这也因为分散练习（distributed practice）而支撑了我的主张：当学生把阅读分散在几天而不是死记硬背时，间隔效应帮助他们更好地记住了科学文章。"},
				},
				Annotations: []models.Annotation{
					{ID: "A1", SentenceID: "S1", ExactQuote: "Breaking study time into spaced, shorter sessions improves how well students remember material over the long run.", Category: "claim", PartID: "part_a", Badge: "Part A · Claim (+1)", CommentZh: "主张明确回应题干"},
					{ID: "A2", SentenceID: "S2", ExactQuote: "According to Source 1, medical trainees who learned surgical suturing in four weekly two-hour sessions scored higher one month later", Category: "evidence", PartID: "part_b_i", Badge: "B(i) · Source 1 (+1)", CommentZh: "句首 According to Source 1 标注有效，结果方向正确"},
					{ID: "A3", SentenceID: "S3", ExactQuote: "spacing effect, because spreading practice out gives the brain time to consolidate the skill", Category: "concept", PartID: "part_b_ii", Badge: "B(ii) · Spacing Effect (+2)", CommentZh: "正确运用 Spacing Effect 解释长时记忆保持", ConceptCheck: &models.ConceptCheck{Term: "spacing effect", CanonicalTerm: "Spacing Effect (Distributed Practice)", CEDUnit: "Unit 2: Cognition", InAPCED: true, AppliedProperly: true, NoteZh: "首次使用有效得分"}},
					{ID: "A4", SentenceID: "S4", ExactQuote: "students who read four science articles over two separate days kept a 73% score a week later", Category: "issue", PartID: "part_c_i", Badge: "C(i) · 漏标 Source 出处 (0分)", CommentZh: "数据确系 Source 2，但整段未写 Source 2 字样，按 CB 细则无 Citation 不得证据分"},
					{ID: "A5", SentenceID: "S5", ExactQuote: "because of distributed practice: when students space their reading across days rather than cramming, the spacing effect", Category: "issue", PartID: "part_c_ii", Badge: "C(ii) · 概念同义重复 (仅得1分)", CommentZh: "Distributed practice 与 B(ii) 的 Spacing effect 属于同一考纲概念，C(ii) 封顶 1 分", ConceptCheck: &models.ConceptCheck{Term: "distributed practice", CanonicalTerm: "Spacing Effect (Distributed Practice)", CEDUnit: "Unit 2: Cognition", InAPCED: true, AppliedProperly: true, DuplicateWithPartB: true, NoteZh: "与 B(ii) 命中同一 CED 词条，无法拿第二个概念分"}},
				},
				RubricParts: []models.RubricPartScore{
					{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确主张", MaxScore: 1, AIScore: 1, ReasonZh: "S1 清晰提出分时段间隔学习有助于长期记忆。"},
					{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "具体研究发现 + 明确文献标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 1", ReasonZh: "准确引述 Source 1 外科缝合实验结果并标注出处。"},
					{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "解释支撑关系 + 运用心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 1", AppliedConcept: "Spacing Effect (Distributed Practice)", ReasonZh: "解释证据与主张关联（+1），并正确运用 Spacing Effect（+1）。"},
					{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "另一篇不同文献的具体发现 + 标注", MaxScore: 1, AIScore: 0, CitedSource: "", Flags: []string{"missing_citation"}, ReasonZh: "虽然引述了 Source 2 的 73% vs 54% 数据，但全段未标注 (Source 2)，按官方评分细则无出处得 0 分。"},
					{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "解释支撑关系 + 第二个不同心理学概念", MaxScore: 2, AIScore: 1, CitedSource: "Source 2", AppliedConcept: "Spacing Effect (Distributed Practice)", Flags: []string{"duplicate_concept"}, ReasonZh: "虽 C(i) 漏标出处，按解耦规则仍可获解释分（+1）；但 Distributed Practice 与 B(ii) 的 Spacing Effect 属于同一概念，扣第 2 分。"},
				},
			},
		},
		{
			name:   "Maya Patel",
			code:   "AP-204931",
			pages:  "P.3",
			status: "ai_graded",
			text: `Short, focused study sessions spaced over time allow students to remember information much more effectively than long passive sessions.

In Source 3, high school biology students who completed one total hour of short self-quizzing modules across two weeks scored 78.4 on a national exam, matching students who sat through 23 hours of traditional lecture review (Source 3). This supports my claim because self-quizzing uses the testing effect, where actively pulling answers from memory strengthens neural retrieval pathways far more efficiently than passive listening.

Similarly, in Source 1, surgical residents who practiced over four weekly sessions had higher mean scores a month later than the single-day workshop group (Source 1). This supports my claim because retrieval and statistical significance show that the residents were able to rehearse the sutures better. Also, chunking means grouping items into seven plus or minus two units.`,
			result: models.GradingResult{
				AITotalScore:  5,
				MaxTotalScore: 7,
				GraderMode:    "calibration_seed",
				ModelUsed:     "deepseek-flash",
				SummaryZh:     "禁用词与纯定义陷阱卷（5/7）：A + B(i) + B(ii) + C(i) 拿满 5 分；C(ii) 先后踩中题干禁用词黑名单（`retrieval`、`statistical significance`、`rehearsal` 均在 CB 禁用列表内），最后一句又孤立背诵 `chunking` 定义而未结合缝合实验，故 C(ii) 仅得 1 分解释分。",
				Sentences: []models.SentencePair{
					{ID: "S1", En: "Short, focused study sessions spaced over time allow students to remember information much more effectively than long passive sessions.", Zh: "在时间上分散的短时专注学习，比长时间的被动学习能让学生更有效地记住信息。"},
					{ID: "S2", En: "In Source 3, high school biology students who completed one total hour of short self-quizzing modules across two weeks scored 78.4 on a national exam, matching students who sat through 23 hours of traditional lecture review (Source 3).", Zh: "在 Source 3 中，两周内完成总计一小时短时自测模块的高中生物学生，在全国考试中拿到 78.4 分，与上了 23 小时传统讲授复习课的学生表现相当（Source 3）。"},
					{ID: "S3", En: "This supports my claim because self-quizzing uses the testing effect, where actively pulling answers from memory strengthens neural retrieval pathways far more efficiently than passive listening.", Zh: "这支撑了我的主张，因为自测运用了测试效应（testing effect），主动从记忆中提取答案比被动听讲能更高效地强化神经提取通路。"},
					{ID: "S4", En: "Similarly, in Source 1, surgical residents who practiced over four weekly sessions had higher mean scores a month later than the single-day workshop group (Source 1).", Zh: "类似地，在 Source 1 中，分四周练习的外科住院医在一个月后的平均分高于单日工作坊组（Source 1）。"},
					{ID: "S5", En: "This supports my claim because retrieval and statistical significance show that the residents were able to rehearse the sutures better.", Zh: "这支撑了我的主张，因为提取（retrieval）和统计显著性（statistical significance）表明住院医能够更好地复述演练（rehearse）缝合操作。"},
					{ID: "S6", En: "Also, chunking means grouping items into seven plus or minus two units.", Zh: "此外，组块化（chunking）的意思是把项目归类成七加减二个单元。"},
				},
				Annotations: []models.Annotation{
					{ID: "A1", SentenceID: "S1", ExactQuote: "Short, focused study sessions spaced over time allow students to remember information much more effectively", Category: "claim", PartID: "part_a", Badge: "Part A · Claim (+1)", CommentZh: "明确提出短时分散学习优于长时被动学习"},
					{ID: "A2", SentenceID: "S2", ExactQuote: "completed one total hour of short self-quizzing modules across two weeks scored 78.4 on a national exam, matching students who sat through 23 hours", Category: "evidence", PartID: "part_b_i", Badge: "B(i) · Source 3 (+1)", CommentZh: "准确引述 Source 3 对比结果并标注出处"},
					{ID: "A3", SentenceID: "S3", ExactQuote: "self-quizzing uses the testing effect, where actively pulling answers from memory strengthens neural retrieval pathways", Category: "concept", PartID: "part_b_ii", Badge: "B(ii) · Testing Effect (+2)", CommentZh: "Testing Effect 不在禁用列表内，且准确解释了自测为何以 1 小时匹敌 23 小时", ConceptCheck: &models.ConceptCheck{Term: "testing effect", CanonicalTerm: "Testing Effect (Retrieval Practice)", CEDUnit: "Unit 2: Cognition", InAPCED: true, AppliedProperly: true, NoteZh: "注意：虽然裸词 retrieval 被禁，但 Testing Effect 属于受认可的独立心理学效应"}},
					{ID: "A4", SentenceID: "S4", ExactQuote: "surgical residents who practiced over four weekly sessions had higher mean scores a month later than the single-day workshop group (Source 1)", Category: "evidence", PartID: "part_c_i", Badge: "C(i) · Source 1 (+1)", CommentZh: "跨源选用 Source 1，结果准确"},
					{ID: "A5", SentenceID: "S5", ExactQuote: "retrieval and statistical significance show that the residents were able to rehearse", Category: "issue", PartID: "part_c_ii", Badge: "C(ii) · 误踩题干禁用词", CommentZh: "retrieval、statistical significance、rehearsal 均在本题 CB 禁用概念黑名单内，不计概念分", ConceptCheck: &models.ConceptCheck{Term: "retrieval / statistical significance", CanonicalTerm: "Ineligible Source Term", CEDUnit: "Banned by Scoring Guideline", InAPCED: false, IsIneligible: true, AppliedProperly: false, NoteZh: "官方细则明确规定：retrieval, rehearsal, statistical significance 不可用于拿应用分"}},
					{ID: "A6", SentenceID: "S6", ExactQuote: "chunking means grouping items into seven plus or minus two units.", Category: "issue", PartID: "part_c_ii", Badge: "C(ii) · 纯背定义未结合材料", CommentZh: "孤立默写 Chunking 定义，完全未关联缝合训练，按 Apply don't define 规则得 0 应用分", ConceptCheck: &models.ConceptCheck{Term: "chunking", CanonicalTerm: "Chunking", CEDUnit: "Unit 2: Cognition", InAPCED: true, IsIneligible: false, AppliedProperly: false, NoteZh: "虽在考纲内，但仅罗列定义（definition alone），未结合材料应用"}},
				},
				RubricParts: []models.RubricPartScore{
					{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确主张", MaxScore: 1, AIScore: 1, ReasonZh: "主张清晰具体。"},
					{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "具体研究发现 + 明确文献标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 3", ReasonZh: "准确引述 Source 3 数据并标注。"},
					{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "解释支撑关系 + 运用心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 3", AppliedConcept: "Testing Effect (Retrieval Practice)", ReasonZh: "准确运用 Testing Effect 解释 1 小时自测为何能匹敌 23 小时听课。"},
					{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "另一篇不同文献的具体发现 + 标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 1", ReasonZh: "换用 Source 1 且证据准确。"},
					{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "解释支撑关系 + 第二个不同心理学概念", MaxScore: 2, AIScore: 0, CitedSource: "Source 1", AppliedConcept: "retrieval (ineligible) / chunking (definition only)", Flags: []string{"ineligible_concept", "definition_only"}, ReasonZh: "S5 依赖统计显著性同义反复而未解释机制，且所提 retrieval/rehearsal 属禁用词，S6 的 chunking 仅为孤立定义，得 0 分。"},
				},
			},
		},
		{
			name:   "Lucas Wang",
			code:   "AP-204944",
			pages:  "P.4",
			status: "ai_graded",
			text: `What strengthens a person's ability to retrieve newly learned information? It is very important for medical students and college students to study hard.

However, looking at Source 2, students who spaced their reading of four science articles across two sessions 48 hours apart kept a 73% score after one week, whereas students who crammed all four articles in one afternoon fell to 54% (Source 2). This shows that spacing study sessions across days strengthens long-term retention because it prevents fatigue and relies on the spacing effect to let each article encode deeply before the next session.

Also, in Source 1, residents who practiced surgical sutures across four weekly sessions outperformed the single-day group one month later (Source 1). Separating practice by a week allows long-term potentiation to strengthen the synaptic pathways needed for motor skill mastery, supporting the idea that spaced schedules produce more durable memory.`,
			result: models.GradingResult{
				AITotalScore:  6,
				MaxTotalScore: 7,
				GraderMode:    "calibration_seed",
				ModelUsed:     "deepseek-flash",
				SummaryZh:     "隐含论点救济示范卷（6/7）：开头仅复述题干问句 + 泛泛说“要努力学习”（Part A = 0 分）；但触发官方【Implicit Claim Rescue 救济条款】——从 B 段推理中可清晰提取出“分散间隔学习能增强长期记忆”的隐含主张，因此 B(i)、B(ii)、C(i)、C(ii) 全额获救拿满 6 分！",
				Sentences: []models.SentencePair{
					{ID: "S1", En: "What strengthens a person's ability to retrieve newly learned information?", Zh: "什么能增强一个人提取新学信息的能力？"},
					{ID: "S2", En: "It is very important for medical students and college students to study hard.", Zh: "对医学生和大学生来说，努力学习是非常重要的。"},
					{ID: "S3", En: "However, looking at Source 2, students who spaced their reading of four science articles across two sessions 48 hours apart kept a 73% score after one week, whereas students who crammed all four articles in one afternoon fell to 54% (Source 2).", Zh: "不过看 Source 2，把四篇科学文章分成相隔 48 小时的两次来读的学生在一周后保持了 73% 的分数，而一个下午全塞完的学生跌到了 54%（Source 2）。"},
					{ID: "S4", En: "This shows that spacing study sessions across days strengthens long-term retention because it prevents fatigue and relies on the spacing effect to let each article encode deeply before the next session.", Zh: "这表明把学习分散在多天进行能增强长期保持，因为它避免了疲劳，并依靠间隔效应让每篇文章在下一次学习前得到深度编码。"},
					{ID: "S5", En: "Also, in Source 1, residents who practiced surgical sutures across four weekly sessions outperformed the single-day group one month later (Source 1).", Zh: "此外在 Source 1 中，分四周练习外科缝合的住院医在一个月后表现优于单日组（Source 1）。"},
					{ID: "S6", En: "Separating practice by a week allows long-term potentiation to strengthen the synaptic pathways needed for motor skill mastery, supporting the idea that spaced schedules produce more durable memory.", Zh: "将练习隔开一周能让长时程增强（long-term potentiation）强化掌握运动技能所需的突触通路，从而支撑了分散安排能产生更持久记忆的观点。"},
				},
				Annotations: []models.Annotation{
					{ID: "A1", SentenceID: "S1", ExactQuote: "What strengthens a person's ability to retrieve newly learned information?", Category: "issue", PartID: "part_a", Badge: "Part A · 仅复述题干 (0分)", CommentZh: "照抄题目问句 + 泛泛空话 study hard，官方细则点名示例 0 分"},
					{ID: "A2", SentenceID: "S4", ExactQuote: "spacing study sessions across days strengthens long-term retention", Category: "claim", PartID: "part_b_ii", Badge: "隐含 Claim 救济生效", CommentZh: "根据 CB General Note 7：Part A 失分时可由 Part B 确立 intended claim 供后续各问判分"},
					{ID: "A3", SentenceID: "S3", ExactQuote: "spaced their reading of four science articles across two sessions 48 hours apart kept a 73% score after one week... (Source 2)", Category: "evidence", PartID: "part_b_i", Badge: "B(i) · Source 2 (+1)", CommentZh: "引用准确，标注清晰"},
					{ID: "A4", SentenceID: "S4", ExactQuote: "relies on the spacing effect to let each article encode deeply", Category: "concept", PartID: "part_b_ii", Badge: "B(ii) · Spacing Effect (+2)", CommentZh: "正确运用间隔效应支撑隐含主张", ConceptCheck: &models.ConceptCheck{Term: "spacing effect", CanonicalTerm: "Spacing Effect (Distributed Practice)", CEDUnit: "Unit 2: Cognition", InAPCED: true, AppliedProperly: true, NoteZh: "考纲概念运用得当"}},
					{ID: "A5", SentenceID: "S5", ExactQuote: "residents who practiced surgical sutures across four weekly sessions outperformed the single-day group one month later (Source 1)", Category: "evidence", PartID: "part_c_i", Badge: "C(i) · Source 1 (+1)", CommentZh: "更换为 Source 1，与隐含主张内部一致"},
					{ID: "A6", SentenceID: "S6", ExactQuote: "allows long-term potentiation to strengthen the synaptic pathways needed for motor skill mastery", Category: "concept", PartID: "part_c_ii", Badge: "C(ii) · LTP 神经机制 (+2)", CommentZh: "跨单元调用 Unit 1 Long-Term Potentiation 解释动作技能突触强化，非常漂亮", ConceptCheck: &models.ConceptCheck{Term: "long-term potentiation", CanonicalTerm: "Long-Term Potentiation (LTP)", CEDUnit: "Unit 1: Biological Bases", InAPCED: true, AppliedProperly: true, NoteZh: "与 B 段概念完全不同，获满分"}},
				},
				RubricParts: []models.RubricPartScore{
					{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确主张", MaxScore: 1, AIScore: 0, Flags: []string{"oversimplified_claim"}, ReasonZh: "S1 照抄题干设问，S2 “study hard” 属于官方点名的过度简化空话，Part A 得 0 分。"},
					{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "具体研究发现 + 明确文献标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 2", Flags: []string{"implicit_claim_rescued"}, ReasonZh: "触发隐含主张救济（S4 确立分散学习增强保持），Source 2 引用准确，得 1 分。"},
					{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "解释支撑关系 + 运用心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 2", AppliedConcept: "Spacing Effect (Distributed Practice)", ReasonZh: "围绕隐含主张完成推理解释并运用 Spacing Effect，得 2 分。"},
					{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "另一篇不同文献的具体发现 + 标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 1", ReasonZh: "换用 Source 1 且与 Part B 隐含主张内部自洽，得 1 分。"},
					{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "解释支撑关系 + 第二个不同心理学概念", MaxScore: 2, AIScore: 2, CitedSource: "Source 1", AppliedConcept: "Long-Term Potentiation (LTP)", ReasonZh: "换用 Unit 1 神经突触长时程增强（LTP）解释运动技能保持，得 2 分。"},
				},
			},
		},
		{
			name:   "Noah Brooks",
			code:   "AP-204952",
			pages:  "P.5",
			status: "ai_graded",
			text: `Cramming all learning into a single long session is the best way to improve long-term memory retention after several weeks.

In Source 1, Group 1 who did the single 8-hour workshop scored much higher one month later than Group 2 who split suturing into four weekly sessions (Source 1). This proves cramming works because of the recency effect, since information stays in working memory for a month.

In Source 2, Group 1 who read all four science articles in one afternoon scored higher on the immediate test (82% vs. 74%) (Source 2). This happens because proactive interference is when old memories block new ones, and proactive interference is also when brand-new memories erase old ones, helping Group 1 remember everything.`,
			result: models.GradingResult{
				AITotalScore:  2,
				MaxTotalScore: 7,
				GraderMode:    "calibration_seed",
				ModelUsed:     "deepseek-flash",
				SummaryZh:     "事实说反与自相矛盾熔断卷（2/7）：Part A 本身虽与主流结论相反但作为可检验主张仍得 1 分；B 段把 Source 1 组间高低彻底说反（0+0 分）；C(i) 引用 Source 2 即时测验 82%>74% 方向属实得 1 分，但 C(ii) 把前摄抑制正反定义同时写了一遍触发【自相矛盾熔断规则】归零。",
				Sentences: []models.SentencePair{
					{ID: "S1", En: "Cramming all learning into a single long session is the best way to improve long-term memory retention after several weeks.", Zh: "把所有学习塞进单次长时间课程里，是提升数周后长时记忆保持的最佳方式。"},
					{ID: "S2", En: "In Source 1, Group 1 who did the single 8-hour workshop scored much higher one month later than Group 2 who split suturing into four weekly sessions (Source 1).", Zh: "在 Source 1 中，参加单日 8 小时工作坊的第 1 组在一个月后的得分远高于分四周练习缝合的第 2 组（Source 1）。"},
					{ID: "S3", En: "This proves cramming works because of the recency effect, since information stays in working memory for a month.", Zh: "这证明突击学习有效，原因是近因效应（recency effect），因为信息会在工作记忆里留存一个月。"},
					{ID: "S4", En: "In Source 2, Group 1 who read all four science articles in one afternoon scored higher on the immediate test (82% vs. 74%) (Source 2).", Zh: "在 Source 2 中，一个下午读完四篇科学文章的第 1 组在即时测试中得分更高（82% 对 74%）（Source 2）。"},
					{ID: "S5", En: "This happens because proactive interference is when old memories block new ones, and proactive interference is also when brand-new memories erase old ones, helping Group 1 remember everything.", Zh: "发生这种情况是因为前摄抑制是指旧记忆阻碍新记忆，同时前摄抑制也是指全新的记忆抹掉旧记忆，从而帮助第 1 组记住所有内容。"},
				},
				Annotations: []models.Annotation{
					{ID: "A1", SentenceID: "S1", ExactQuote: "Cramming all learning into a single long session is the best way to improve long-term memory retention", Category: "claim", PartID: "part_a", Badge: "Part A · Claim (+1)", CommentZh: "按 CB 规则：Part A 独立判分，即使后续难以证成，只要提出明确效应主张即给 1 分"},
					{ID: "A2", SentenceID: "S2", ExactQuote: "Group 1 who did the single 8-hour workshop scored much higher one month later than Group 2", Category: "issue", PartID: "part_b_i", Badge: "B(i) · 数据方向说反 (0分)", CommentZh: "原文是分散组 31.60 > 集中组 27.63，此处将高低关系完全颠倒，B(i) 与 B(ii) 双杀"},
					{ID: "A3", SentenceID: "S4", ExactQuote: "Group 1 who read all four science articles in one afternoon scored higher on the immediate test (82% vs. 74%) (Source 2)", Category: "evidence", PartID: "part_c_i", Badge: "C(i) · Source 2 (+1)", CommentZh: "即时测试确实是集中组 82% > 74%（尽管忽略了一周后暴跌，但单看该条事实陈述属实）"},
					{ID: "A4", SentenceID: "S5", ExactQuote: "proactive interference is when old memories block new ones, and proactive interference is also when brand-new memories erase old ones", Category: "issue", PartID: "part_c_ii", Badge: "C(ii) · 自相矛盾熔断 (0分)", CommentZh: "官方细则经典反例：把 proactive interference 正反两种方向同时写上押宝，触发自相矛盾归零", ConceptCheck: &models.ConceptCheck{Term: "proactive interference", CanonicalTerm: "Proactive Interference", CEDUnit: "Unit 2: Cognition", InAPCED: true, AppliedProperly: false, NoteZh: "触发 Contradiction Rule：正反矛盾定义直接清零"}},
				},
				RubricParts: []models.RubricPartScore{
					{PartID: "part_a", Label: "Part A · Claim", Subtitle: "针对题干提出明确主张", MaxScore: 1, AIScore: 1, ReasonZh: "提出了针对题干的明确立场主张（按独立判分原则获 1 分）。"},
					{PartID: "part_b_i", Label: "Part B(i) · Evidence 1", Subtitle: "具体研究发现 + 明确文献标注", MaxScore: 1, AIScore: 0, CitedSource: "Source 1", Flags: []string{"direction_reversed"}, ReasonZh: "将 Source 1 的实验结果说反（实际 Group 2 更高），得 0 分。"},
					{PartID: "part_b_ii", Label: "Part B(ii) · Reasoning & Concept 1", Subtitle: "解释支撑关系 + 运用心理学概念", MaxScore: 2, AIScore: 0, CitedSource: "Source 1", AppliedConcept: "Recency Effect", Flags: []string{"direction_reversed"}, ReasonZh: "建立在错误歪曲的实验结果之上，且声称工作记忆维持一个月属科学错误，得 0 分。"},
					{PartID: "part_c_i", Label: "Part C(i) · Evidence 2", Subtitle: "另一篇不同文献的具体发现 + 标注", MaxScore: 1, AIScore: 1, CitedSource: "Source 2", ReasonZh: "引述 Source 2 即时测验 82% > 74% 数据方向属实且标注清晰（虽未支撑“数周后长期保持”，此处建议老师人工复核是否扣减）。"},
					{PartID: "part_c_ii", Label: "Part C(ii) · Reasoning & Concept 2", Subtitle: "解释支撑关系 + 第二个不同心理学概念", MaxScore: 2, AIScore: 0, CitedSource: "Source 2", AppliedConcept: "Proactive Interference", Flags: []string{"self_contradiction"}, ReasonZh: "同时将前摄抑制定义为旧干扰新和新干扰旧，触发 CB 自相矛盾条款，且即时测验无法支撑数周后长期保持，得 0 分。"},
				},
			},
		},
	}

	now := nowISO()
	for _, p := range papers {
		resBytes, _ := json.Marshal(p.result)
		if _, err := s.DB.Exec(`
INSERT INTO submissions(problem_set_id, student_name, student_code, page_range, raw_text, status, ai_score, result_json, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ebq.ID, p.name, p.code, p.pages, p.text, p.status, p.result.AITotalScore, string(resBytes), now, now); err != nil {
			return err
		}
	}

	// Also seed one AAQ template problem set so AAQ support is immediately ready
	aaq := &models.ProblemSet{
		Title:        "2026 CB Official Template · Phone Separation & FOMO Anxiety (AAQ)",
		QuestionType: "AAQ",
		PromptText:   "Analyze the summarized study on cellphone availability and state anxiety across Parts A–F (Research Method, Operational Variable, Statistic Interpretation, Ethical Guideline, Generalizability, and Argumentation on whether FOMO causes anxiety).",
		Sources: []models.SourceDoc{
			{
				ID:      "Article",
				Label:   "Summarized Study",
				Title:   "Cheever et al. — Cellphone Presence, Ringing Alerts, and State Anxiety",
				Summary: "163 undergraduate students (72.7% women, 52.3% White) at a private US university were randomly assigned after IRB approval and informed consent (including right to withdraw) to either keep their silenced phone on the desk (Phone Present) or hand it to researchers (Phone Absent). Researchers secretly called participants' phones and later measured anxiety on a 20-item state anxiety scale (1–4). Phone Absent participants reported significantly higher mean anxiety (M = 41.39) than Phone Present participants (M = 31.00, p < .01). At the end, participants were debriefed about the staged call.",
			},
		},
		IneligibleConcepts: []string{},
		ScoringNotes:       "6-part AAQ rubric (Parts A–E 1 pt each, Part F Argumentation 2 pts = 7 pts total).",
	}
	return s.CreateProblemSet(aaq)
}
