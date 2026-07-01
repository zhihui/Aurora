import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Loose search match. The query is split into whitespace-separated tokens;
 * every token must appear in `target` as a substring, and the tokens must
 * occur in order. So "fro d" matches "front design" (fro→front, then d→design),
 * while "d fro" does not. Empty/whitespace queries match everything.
 */
export function fuzzyMatch(query: string, target: string): boolean {
  const tokens = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (tokens.length === 0) return true
  const haystack = target.toLowerCase()
  let from = 0
  for (const token of tokens) {
    const idx = haystack.indexOf(token, from)
    if (idx === -1) return false
    from = idx + token.length
  }
  return true
}
