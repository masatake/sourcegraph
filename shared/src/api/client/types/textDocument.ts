import { Position } from '@sourcegraph/extension-api-types'
import minimatch from 'minimatch'
import { DocumentFilter, DocumentSelector, TextDocument } from 'sourcegraph'

/**
 * A literal to identify a text document in the client.
 */
export interface TextDocumentIdentifier {
    /**
     * The text document's URI.
     */
    uri: string
}

/**
 * Returns whether any of the document selectors match (or "select") the document.
 */
export function match(
    selectors: DocumentSelector | IterableIterator<DocumentSelector>,
    document: Pick<TextDocument, 'uri' | 'languageId'>
): boolean {
    for (const selector of isSingleDocumentSelector(selectors) ? [selectors] : selectors) {
        if (match1(selector, document)) {
            return true
        }
    }
    return false
}

function isSingleDocumentSelector(
    value: DocumentSelector | IterableIterator<DocumentSelector>
): value is DocumentSelector {
    return Array.isArray(value) && (value.length === 0 || isDocumentSelectorElement(value[0]))
}

function isDocumentSelectorElement(value: any): value is DocumentSelector[0] {
    return typeof value === 'string' || isDocumentFilter(value)
}

function isDocumentFilter(value: any): value is DocumentFilter {
    const candidate: DocumentFilter = value
    return (
        typeof candidate.language === 'string' ||
        typeof candidate.scheme === 'string' ||
        typeof candidate.pattern === 'string'
    )
}

function match1(selector: DocumentSelector, document: Pick<TextDocument, 'uri' | 'languageId'>): boolean {
    return score(selector, document.uri, document.languageId) !== 0
}

/**
 * Returns the score that indicates "how well" the document selector matches a document (by its URI and language
 * ID). A higher score indicates a more specific match. The score is a heuristic.
 *
 * For example, a document selector ['*'] matches all documents, so it is not a very specific match for any
 * document (but it *does* match all documents). Its score will be lower than a more specific match, such as the
 * document selector [{language: 'python'}] against a Python document.
 *
 * Taken from
 * https://github.com/Microsoft/vscode/blob/3d35801127f0a62d58d752bc613506e836c5d120/src/vs/editor/common/modes/languageSelector.ts#L24.
 */
export function score(selector: DocumentSelector, candidateUri: string, candidateLanguage: string): number {
    // array -> take max individual value
    let ret = 0
    for (const filter of selector) {
        const value = score1(filter, candidateUri, candidateLanguage)
        if (value === 10) {
            return value // already at the highest
        }
        if (value > ret) {
            ret = value
        }
    }
    return ret
}

function score1(selector: DocumentSelector[0], candidateUri: string, candidateLanguage: string): number {
    if (typeof selector === 'string') {
        // Shorthand notation: "mylang" -> {language: "mylang"}, "*" -> {language: "*""}.
        if (selector === '*') {
            return 5
        }
        if (selector === candidateLanguage) {
            return 10
        }
        return 0
    }

    const { language, scheme, pattern } = selector
    if (!language && !scheme && !pattern) {
        // `{}` was passed as a document filter, treat it like a wildcard
        return 5
    }
    let ret = 0
    if (scheme) {
        if (candidateUri.startsWith(scheme + ':')) {
            ret = 10
        } else if (scheme === '*') {
            ret = 5
        } else {
            return 0
        }
    }
    if (language) {
        if (language === candidateLanguage) {
            ret = 10
        } else if (language === '*') {
            ret = Math.max(ret, 5)
        } else {
            return 0
        }
    }
    if (pattern) {
        if (pattern === candidateUri || candidateUri.endsWith(pattern) || minimatch(candidateUri, pattern)) {
            ret = 10
        } else if (minimatch(candidateUri, '**/' + pattern)) {
            ret = 5
        } else {
            return 0
        }
    }
    return ret
}

/**
 * Convert a character offset in text to the equivalent position.
 */
export function offsetToPosition(text: string, offset: number): Position {
    if (offset <= 0) {
        return { line: 0, character: 0 }
    }
    const before = text.slice(0, offset)
    const newLines = before.match(/\n/g)
    const line = newLines ? newLines.length : 0
    const pre = before.match(/(^|\n).*$/g)
    return { line, character: pre ? pre[0].length + (line === 0 ? 0 : -1) : 0 }
}

/**
 * Convert a position in text to the equivalent character offset.
 */
export function positionToOffset(text: string, pos: Position): number {
    if (pos.line === 0) {
        return pos.character
    }
    let line = 0
    let lastNewLineOffset = -1
    do {
        if (pos.line === line) {
            return lastNewLineOffset + 1 + pos.character
        }
        lastNewLineOffset = text.indexOf('\n', lastNewLineOffset + 1)
        line++
    } while (lastNewLineOffset >= 0)
    return text.length
}
