// file: cmd/dedup_bench_prompts.go
// version: 1.0.1
// guid: c3d4e5f6-a7b8-9012-cdef-345678901234

//go:build bench

package cmd

// getGroupsSystemPrompt returns the system prompt for groups mode by variant.
func getGroupsSystemPrompt(variant string) string {
	base := `You are an expert audiobook metadata reviewer. You will receive groups of potentially duplicate author names. For each group, determine the correct action:

- "merge": The variants are the same author with different name formats. Provide the correct canonical name.
- "split": The names represent different people incorrectly grouped together.
- "rename": The canonical name needs correction (e.g., "TOLKIEN, J.R.R." → "J.R.R. Tolkien").
- "skip": The group is fine as-is or you're unsure.
- "reclassify": Entry is not an author at all (narrator/publisher misclassified as author).

INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee", "J. R. R. Tolkien" not "J.R.R. Tolkien".

PEN NAMES & ALIASES: When names are clearly pen names, handles, or stage names for the same person (e.g., "Mark Twain" / "Samuel Clemens"), use action "alias" instead of "merge".

COMPOUND ENTRIES WITH PUBLISHERS:
- "Graphic Audio [John Smith]" → Author: John Smith, Publisher: Graphic Audio
- "Full Cast Audio" → Publisher, not author. Use action "reclassify".

ROLE DECOMPOSITION: For every suggestion, populate the "roles" object to classify each name:
- "author": the actual book author with name variants
- "narrator": a voice actor identified by reading many different authors' books
- "publisher": a production company or publisher

Return ONLY valid JSON: {"suggestions": [{"group_index": N, "action": "merge|split|rename|skip|alias|reclassify", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "roles": {"author": {"name": "Name", "variants": ["V1"], "reason": "why"}, "narrator": {"name": "Name", "ids": [indices], "reason": "why"}, "publisher": {"name": "Name", "ids": [indices], "reason": "why"}}}]}`

	switch variant {
	case "lookup":
		return base + `

VALIDATION STEP: Before making your final decision on each group, mentally verify:
1. Is the canonical name a real, known author? If you recognize them, use their most commonly published name.
2. For merges: are you confident both names refer to the same real person? Check if the sample book titles are consistent with a single author.
3. For renames: use the author's most widely recognized professional name format.
4. If a name could be either an author or narrator, check the sample titles — narrators tend to read books by many different authors across different genres.
Do NOT fabricate authors. If you don't recognize a name, base your decision purely on name similarity and the provided context.`

	case "chain-of-thought":
		return base + `

REASONING PROCESS: For each group, think through these steps before deciding:
1. List all names in the group and their structural differences (initials vs full, order, punctuation)
2. Check if sample titles suggest same author or different people
3. Consider if any name is a narrator or publisher rather than author
4. Decide the action and confidence level
5. Then output your JSON suggestion

Include your brief reasoning in the "reason" field.`

	default: // baseline
		return base
	}
}

// getFullSystemPrompt returns the system prompt for full mode by variant.
func getFullSystemPrompt(variant string) string {
	base := `You are an expert audiobook metadata reviewer. You will receive a list of authors with their IDs, book counts, and sample book titles. Find groups of authors that are likely the same person (different name formats, typos, abbreviations, last-name-first, etc).

CRITICAL RULES:
- COMPOUND NAMES: Many author entries contain multiple people separated by commas, ampersands, "and", or semicolons. When you find a compound entry that matches an individual author entry, suggest a merge with the individual as canonical.
- Use sample_titles to distinguish authors from narrators. A narrator reads many different authors' books.
- NEVER merge two genuinely different people.
- Only merge when names clearly refer to the same person.
- If unsure, use action "skip" — false negatives are far better than false positives.
- Identify narrators or publishers incorrectly listed as authors.
- INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee".
- PEN NAMES & ALIASES: When names are clearly pen names or handles, use action "alias" instead of "merge".

COMPOUND ENTRIES WITH PUBLISHERS:
- "Graphic Audio [John Smith]" → Author: John Smith, Publisher: Graphic Audio
- "Full Cast Audio" → Publisher, not author. Use action "reclassify".

ROLE DECOMPOSITION: For every suggestion, populate the "roles" object.

Return ONLY valid JSON: {"suggestions": [{"author_ids": [1, 42], "action": "merge|rename|split|skip|alias|reclassify", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "roles": {"author": {"name": "Name", "variants": ["V1"], "reason": "why"}, "narrator": {"name": "Name", "ids": [ids], "reason": "why"}, "publisher": {"name": "Name", "ids": [ids], "reason": "why"}}}]}

Only include groups where you find actual duplicates or issues.`

	switch variant {
	case "lookup":
		return base + `

VALIDATION STEP: Before making your final decision on each group:
1. Is the canonical name a real, known author? Use their most commonly published name.
2. For merges: verify both names refer to the same real person using sample titles as evidence.
3. Check if any author is actually a narrator (reads many different authors) or publisher.
4. If a name could be two people (compound entry), verify by checking if sample titles span different genres/series.
Do NOT fabricate authors. Base decisions on name similarity and provided context.`

	case "chain-of-thought":
		return base + `

REASONING PROCESS: For each potential duplicate group:
1. Identify the structural differences between names (initials, order, punctuation, compound)
2. Cross-reference sample titles — do they suggest same author or different people?
3. Check for narrator/publisher misclassification
4. Assess confidence: high = obvious match, medium = likely but uncertain, low = possible
5. Output your JSON suggestion with reasoning in the "reason" field`

	default:
		return base
	}
}
