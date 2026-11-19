package ced

import "strings"

type ConceptEntry struct {
	Term       string   `json:"term"`
	Unit       string   `json:"unit"`
	Aliases    []string `json:"aliases,omitempty"`
	Definition string   `json:"definition"`
}

// Catalog covers high-frequency concepts across all 5 units of the 2025/2026 AP Psychology CED.
var Catalog = []ConceptEntry{
	// Unit 1: Biological Bases of Behavior
	{Term: "Long-Term Potentiation (LTP)", Unit: "Unit 1: Biological Bases", Aliases: []string{"long-term potentiation", "ltp", "synaptic plasticity", "neuroplasticity"}, Definition: "Strengthening of synaptic connections through repeated neural firing, facilitating memory formation."},
	{Term: "Memory Consolidation", Unit: "Unit 1 / Unit 2: Sleep & Cognition", Aliases: []string{"consolidation", "hippocampal consolidation", "sleep consolidation"}, Definition: "Neural process of stabilizing newly encoded memory traces into durable long-term storage, aided by rest and REM/NREM sleep."},
	{Term: "Hippocampus", Unit: "Unit 1: Biological Bases", Aliases: []string{"hippocampal"}, Definition: "Limbic structure essential for encoding and consolidating explicit declarative memories."},
	{Term: "Amygdala", Unit: "Unit 1: Biological Bases", Aliases: []string{"emotional arousal memory"}, Definition: "Limbic region involved in fear, emotion processing, and emotionally charged memory formation."},
	{Term: "Prefrontal Cortex", Unit: "Unit 1: Biological Bases", Aliases: []string{"executive control", "executive functioning"}, Definition: "Frontal lobe region supporting working memory, planning, impulse inhibition, and decision-making."},
	{Term: "Sympathetic Nervous System", Unit: "Unit 1: Biological Bases", Aliases: []string{"fight-or-flight", "fight or flight", "autonomic arousal"}, Definition: "Division of the autonomic nervous system that mobilizes physiological arousal under stress."},
	{Term: "Parasympathetic Nervous System", Unit: "Unit 1: Biological Bases", Aliases: []string{"rest and digest"}, Definition: "Division of the autonomic nervous system that calms physiological arousal and conserves energy."},
	{Term: "Dopamine Reward Pathway", Unit: "Unit 1: Biological Bases", Aliases: []string{"dopamine", "reward pathway", "nucleus accumbens"}, Definition: "Neurotransmitter system mediating reward prediction, motivation, and reinforcement."},

	// Unit 2: Cognition
	{Term: "Spacing Effect (Distributed Practice)", Unit: "Unit 2: Cognition", Aliases: []string{"spacing effect", "distributed practice", "spaced practice", "massed vs distributed practice", "massed practice"}, Definition: "Memory retention is stronger when study or practice sessions are spread out over time rather than crammed into a single massed session."},
	{Term: "Testing Effect (Retrieval Practice)", Unit: "Unit 2: Cognition", Aliases: []string{"testing effect", "retrieval practice"}, Definition: "Enhanced long-term memory resulting from actively retrieving information rather than passively rereading it."},
	{Term: "Levels of Processing (Deep Processing)", Unit: "Unit 2: Cognition", Aliases: []string{"levels of processing", "deep processing", "semantic encoding", "shallow processing", "elaborative rehearsal"}, Definition: "Encoding information semantically based on meaning produces stronger, more durable recall than shallow structural or phonemic processing."},
	{Term: "Maintenance Rehearsal", Unit: "Unit 2: Cognition", Aliases: []string{"rote repetition"}, Definition: "Repeating information without semantic connection to keep it active in short-term/working memory."},
	{Term: "Working Memory Model", Unit: "Unit 2: Cognition", Aliases: []string{"working memory", "central executive", "phonological loop", "visuospatial sketchpad", "cognitive load"}, Definition: "Active, limited-capacity system that temporarily holds and manipulates information during cognitive tasks."},
	{Term: "Chunking", Unit: "Unit 2: Cognition", Aliases: []string{"chunk", "hierarchies"}, Definition: "Organizing items into familiar, manageable units to expand effective working memory capacity."},
	{Term: "Serial Position Effect", Unit: "Unit 2: Cognition", Aliases: []string{"serial position", "primacy effect", "recency effect"}, Definition: "Tendency to recall the first (primacy, transferred to LTM) and last (recency, still in WM) items in a list best."},
	{Term: "Primacy Effect", Unit: "Unit 2: Cognition", Aliases: []string{"primacy"}, Definition: "Superior recall of early items in a sequence due to greater encoding into long-term memory."},
	{Term: "Recency Effect", Unit: "Unit 2: Cognition", Aliases: []string{"recency"}, Definition: "Superior immediate recall of final items in a sequence because they remain in working memory."},
	{Term: "Proactive Interference", Unit: "Unit 2: Cognition", Aliases: []string{"proactive"}, Definition: "Forward-acting disruptive effect of prior learning on the recall of new information."},
	{Term: "Retroactive Interference", Unit: "Unit 2: Cognition", Aliases: []string{"retroactive"}, Definition: "Backward-acting disruptive effect of new learning on the recall of old information."},
	{Term: "Context-Dependent Memory", Unit: "Unit 2: Cognition", Aliases: []string{"context-dependent", "encoding specificity", "retrieval cues"}, Definition: "Improved recall when external environmental cues present at encoding match those present at retrieval."},
	{Term: "State-Dependent Memory", Unit: "Unit 2: Cognition", Aliases: []string{"state-dependent", "mood-congruent memory"}, Definition: "Improved recall when internal physiological or emotional states match between encoding and retrieval."},
	{Term: "Explicit (Declarative) Memory", Unit: "Unit 2: Cognition", Aliases: []string{"explicit memory", "declarative memory", "semantic memory", "episodic memory"}, Definition: "Memory of facts and personally experienced events that can be consciously declared."},
	{Term: "Implicit (Procedural) Memory", Unit: "Unit 2: Cognition", Aliases: []string{"implicit memory", "procedural memory", "automatic processing"}, Definition: "Retention of learned motor/cognitive skills and conditioned associations independent of conscious recollection."},
	{Term: "Selective Attention", Unit: "Unit 2: Cognition", Aliases: []string{"selective attention", "divided attention", "inattentional blindness", "cocktail party effect"}, Definition: "Focusing conscious awareness on a particular stimulus while filtering out competing stimuli."},
	{Term: "Schemas (Assimilation & Accommodation)", Unit: "Unit 2: Cognition", Aliases: []string{"schema", "schemas", "assimilation", "accommodation"}, Definition: "Mental frameworks that organize and interpret information, shaping encoding and reconstruction."},
	{Term: "Confirmation Bias", Unit: "Unit 2: Cognition", Aliases: []string{"confirmation bias", "belief perseverance"}, Definition: "Tendency to search for and favor information that confirms preexisting beliefs while ignoring contradictory evidence."},
	{Term: "Availability & Representativeness Heuristics", Unit: "Unit 2: Cognition", Aliases: []string{"availability heuristic", "representativeness heuristic", "framing", "anchoring"}, Definition: "Mental shortcuts that speed judgment based on ease of recall or prototype similarity, sometimes causing systematic bias."},
	{Term: "Metacognition", Unit: "Unit 2: Cognition", Aliases: []string{"metacognitive", "illusion of competence"}, Definition: "Awareness and monitoring of one's own cognitive and learning processes."},
	{Term: "Forgetting Curve (Ebbinghaus)", Unit: "Unit 2: Cognition", Aliases: []string{"forgetting curve", "ebbinghaus", "storage decay"}, Definition: "Rapid initial drop in retention after learning that levels off over time."},

	// Unit 3: Development and Learning
	{Term: "Operant Conditioning (Reinforcement & Punishment)", Unit: "Unit 3: Development & Learning", Aliases: []string{"operant conditioning", "positive reinforcement", "negative reinforcement", "punishment", "schedules of reinforcement"}, Definition: "Associative learning in which behavior is strengthened if followed by a reinforcer or diminished if followed by a punisher."},
	{Term: "Classical Conditioning", Unit: "Unit 3: Development & Learning", Aliases: []string{"classical conditioning", "conditioned response", "extinction", "spontaneous recovery"}, Definition: "Learning to link two stimuli so a neutral stimulus comes to elicit an associative response."},
	{Term: "Social Learning Theory (Observational Learning)", Unit: "Unit 3: Development & Learning", Aliases: []string{"observational learning", "modeling", "bandura", "vicarious reinforcement"}, Definition: "Learning behaviors and expectations by observing and imitating models."},
	{Term: "Zone of Proximal Development & Scaffolding", Unit: "Unit 3: Development & Learning", Aliases: []string{"zone of proximal development", "zpd", "scaffolding", "vygotsky"}, Definition: "Gap between what a learner can accomplish independently versus with guided social support."},
	{Term: "Attachment Styles", Unit: "Unit 3: Development & Learning", Aliases: []string{"secure attachment", "insecure attachment", "anxious attachment", "avoidant attachment", "ainsworth"}, Definition: "Enduring emotional bonds established early in life that shape interpersonal security and stress regulation."},
	{Term: "Psychosocial Stages (Identity vs. Role Confusion)", Unit: "Unit 3: Development & Learning", Aliases: []string{"identity vs role confusion", "erikson", "identity formation"}, Definition: "Erikson's developmental task in adolescence involving exploring roles to forge a coherent sense of self."},

	// Unit 4: Social Psychology and Personality
	{Term: "Yerkes-Dodson Law (Optimal Arousal)", Unit: "Unit 4: Social & Personality", Aliases: []string{"yerkes-dodson", "optimal arousal"}, Definition: "Performance peaks at moderate arousal, with lower optimal arousal for complex or unfamiliar tasks."},
	{Term: "Self-Determination Theory (Intrinsic vs. Extrinsic Motivation)", Unit: "Unit 4: Social & Personality", Aliases: []string{"intrinsic motivation", "extrinsic motivation", "overjustification effect", "self-determination theory"}, Definition: "Motivation driven by autonomy, competence, and relatedness versus external rewards or pressures."},
	{Term: "Self-Efficacy", Unit: "Unit 4: Social & Personality", Aliases: []string{"self-efficacy", "locus of control", "internal locus of control"}, Definition: "One's belief in one's ability to succeed in specific situations or accomplish a task."},
	{Term: "Cognitive Dissonance", Unit: "Unit 4: Social & Personality", Aliases: []string{"cognitive dissonance", "festinger"}, Definition: "Psychological discomfort when actions conflict with attitudes, motivating attitude change."},
	{Term: "Fundamental Attribution Error", Unit: "Unit 4: Social & Personality", Aliases: []string{"fundamental attribution error", "actor-observer bias", "self-serving bias", "situational attribution", "dispositional attribution"}, Definition: "Tendency for observers to overestimate dispositional traits and underestimate situational pressures in others' behavior."},
	{Term: "Normative & Informational Social Influence", Unit: "Unit 4: Social & Personality", Aliases: []string{"normative social influence", "informational social influence", "conformity", "asch"}, Definition: "Conforming either to gain social approval/avoid rejection or to accept others' evidence about reality."},
	{Term: "Social Facilitation & Social Loafing", Unit: "Unit 4: Social & Personality", Aliases: []string{"social facilitation", "social loafing", "deindividuation", "group polarization", "groupthink"}, Definition: "Presence of others strengthens dominant responses on simple tasks or reduces individual effort in pooled group work."},
	{Term: "Bystander Effect & Diffusion of Responsibility", Unit: "Unit 4: Social & Personality", Aliases: []string{"bystander effect", "diffusion of responsibility"}, Definition: "Reduced likelihood of helping when other bystanders are present because responsibility feels shared."},
	{Term: "Social Desirability Bias", Unit: "Unit 4: Social & Personality", Aliases: []string{"social desirability", "self-report bias"}, Definition: "Tendency for survey respondents to answer in ways viewed favorably by others rather than truthfully."},
	{Term: "Big Five Personality Traits", Unit: "Unit 4: Social & Personality", Aliases: []string{"big five", "extraversion", "conscientiousness", "neuroticism", "openness", "agreeableness"}, Definition: "Five-factor trait taxonomy capturing stable dimensions of individual differences."},

	// Unit 5: Mental and Physical Health
	{Term: "General Adaptation Syndrome (GAS)", Unit: "Unit 5: Mental & Physical Health", Aliases: []string{"general adaptation syndrome", "selye", "alarm resistance exhaustion"}, Definition: "Selye's three-phase physiological stress response: alarm, resistance, and exhaustion."},
	{Term: "Cognitive Appraisal & Coping Strategies", Unit: "Unit 5: Mental & Physical Health", Aliases: []string{"problem-focused coping", "emotion-focused coping", "cognitive appraisal", "lazarus"}, Definition: "Evaluating stressors as threats or challenges and addressing them either directly or by regulating emotional distress."},
	{Term: "Cognitive Behavioral Therapy (CBT)", Unit: "Unit 5: Mental & Physical Health", Aliases: []string{"cognitive behavioral therapy", "cbt", "cognitive restructuring"}, Definition: "Evidence-based therapy combining cognitive restructuring of maladaptive beliefs with behavioral change."},
}

func MatchConcept(raw string, ineligible []string) (entry *ConceptEntry, isIneligible bool) {
	norm := strings.ToLower(strings.TrimSpace(raw))
	if norm == "" {
		return nil, false
	}
	for _, bad := range ineligible {
		b := strings.ToLower(strings.TrimSpace(bad))
		if b != "" && (norm == b || strings.Contains(norm, b)) {
			// Guard against banning "retrieval practice" when only bare "retrieval" is banned
			if b == "retrieval" && (strings.Contains(norm, "retrieval practice") || strings.Contains(norm, "testing effect") || strings.Contains(norm, "context-dependent") || strings.Contains(norm, "retrieval cue")) {
				continue
			}
			if b == "rehearsal" && (strings.Contains(norm, "elaborative rehearsal") || strings.Contains(norm, "maintenance rehearsal")) {
				continue
			}
			isIneligible = true
		}
	}
	for i := range Catalog {
		c := &Catalog[i]
		if strings.ToLower(c.Term) == norm {
			return c, isIneligible
		}
		for _, a := range c.Aliases {
			if norm == a || strings.Contains(norm, a) || strings.Contains(a, norm) {
				return c, isIneligible
			}
		}
	}
	return nil, isIneligible
}

func Search(query string) []ConceptEntry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		if len(Catalog) > 25 {
			return Catalog[:25]
		}
		return Catalog
	}
	var out []ConceptEntry
	for _, c := range Catalog {
		if strings.Contains(strings.ToLower(c.Term), q) ||
			strings.Contains(strings.ToLower(c.Unit), q) ||
			strings.Contains(strings.ToLower(c.Definition), q) {
			out = append(out, c)
			continue
		}
		for _, a := range c.Aliases {
			if strings.Contains(a, q) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}
