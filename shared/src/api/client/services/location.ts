import { combineLatest, from, Observable, of } from 'rxjs'
import { map, switchMap } from 'rxjs/operators'
import { ReferenceParams, TextDocumentPositionParams, TextDocumentRegistrationOptions } from '../../protocol'
import { Location } from '../../protocol/plainTypes'
import { Model, ViewComponentData } from '../model'
import { match, TextDocumentIdentifier } from '../types/textDocument'
import { DocumentFeatureProviderRegistry } from './registry'
import { flattenAndCompact } from './util'

/**
 * Function signature for retrieving related locations given a location (e.g., definition, implementation, and type
 * definition).
 */
export type ProvideTextDocumentLocationSignature<
    P extends TextDocumentPositionParams = TextDocumentPositionParams,
    L extends Location = Location
> = (params: P) => Observable<L | L[] | null>

/**
 * Provides location results from matching registered providers for definition, implementation, and type definition
 * requests.
 */
export class TextDocumentLocationProviderRegistry<
    P extends TextDocumentPositionParams = TextDocumentPositionParams,
    L extends Location = Location
> extends DocumentFeatureProviderRegistry<ProvideTextDocumentLocationSignature<P, L>> {
    public getLocation(params: P): Observable<L | L[] | null> {
        return getLocation<P, L>(this.providersForDocument(params.textDocument), params)
    }

    public getLocationsAndProviders(
        model: Observable<Pick<Model, 'visibleViewComponents'>>,
        extraParams: Pick<P, Exclude<keyof P, keyof TextDocumentPositionParams>>
    ): Observable<{ locations: Observable<Location[] | null> | null; hasProviders: boolean }> {
        return combineLatest(this.entries, model).pipe(
            map(([entries, { visibleViewComponents }]) => {
                // TODO!(sqs): add a way to get the focused view component
                if (
                    !visibleViewComponents ||
                    visibleViewComponents.length === 0 ||
                    visibleViewComponents[0].selections.length === 0
                ) {
                    return { locations: null, hasProviders: false }
                }

                const providers = entries
                    .filter(({ registrationOptions }) =>
                        match(registrationOptions.documentSelector, visibleViewComponents[0].item)
                    )
                    .map(({ provider }) => provider)
                return {
                    locations: getLocations<P, L>(of(providers), {
                        textDocument: visibleViewComponents[0].item,
                        position: visibleViewComponents[0].selections[0].start,
                        ...extraParams,
                    }),
                    hasProviders: providers.length > 0,
                }
            })
        )
    }
}

/**
 * Returns an observable that emits the providers' location results whenever any of the last-emitted set of
 * providers emits hovers.
 *
 * Most callers should use the TextDocumentLocationProviderRegistry class, which uses the registered providers.
 */
export function getLocation<
    P extends TextDocumentPositionParams = TextDocumentPositionParams,
    L extends Location = Location
>(providers: Observable<ProvideTextDocumentLocationSignature<P, L>[]>, params: P): Observable<L | L[] | null> {
    return getLocations(providers, params).pipe(
        map(results => {
            if (results !== null && results.length === 1) {
                return results[0]
            }
            return results
        })
    )
}

/**
 * Like getLocation, except the returned observable never emits singular values, always either an array or null.
 */
export function getLocations<
    P extends TextDocumentPositionParams = TextDocumentPositionParams,
    L extends Location = Location
>(providers: Observable<ProvideTextDocumentLocationSignature<P, L>[]>, params: P): Observable<L[] | null> {
    return providers.pipe(
        switchMap(providers => {
            if (providers.length === 0) {
                return [null]
            }
            return combineLatest(providers.map(provider => from(provider(params))))
        }),
        map(flattenAndCompact)
    )
}

/**
 * Provides reference results from all providers.
 *
 * Reference results are always an array or null, unlike results from other location providers (e.g., from
 * textDocument/definition), which can be a single item, an array, or null.
 */
export class TextDocumentReferencesProviderRegistry extends TextDocumentLocationProviderRegistry<ReferenceParams> {
    /** Gets reference locations from all matching providers. */
    public getLocation(params: ReferenceParams): Observable<Location[] | null> {
        // References are always an array (unlike other locations, which can be returned as L | L[] |
        // null).
        return getLocations(this.providersForDocument(params.textDocument), params)
    }
}

/**
 * Registration options for a text document provider that has an ID (such as {@link sourcegraph.LocationProvider}).
 */
export interface TextDocumentProviderIDRegistrationOptions extends TextDocumentRegistrationOptions {
    /**
     * The identifier of the provider, used to distinguish it among other providers.
     *
     * This corresponds to, e.g., the `id` parameter in {@link sourcegraph.languages.registerLocationProvider}.
     */
    id: string
}

/**
 * The registry for text document location providers with a distinguishing ID (i.e., registered using
 * {@link TextDocumentProviderIDRegistrationOptions}).
 */
export class TextDocumentLocationProviderIDRegistry extends DocumentFeatureProviderRegistry<
    ProvideTextDocumentLocationSignature<TextDocumentPositionParams, Location>,
    TextDocumentProviderIDRegistrationOptions
> {
    /**
     * @param id The provider ID.
     * @returns an observable of the set of registered providers that apply to the document with the given ID.
     * (Usually there is at most 1 such provider.) The observable emits initially and whenever the set changes (due
     * to a provider being registered or unregistered).
     */
    public providersForDocumentWithID(
        id: string,
        document: TextDocumentIdentifier
    ): Observable<ProvideTextDocumentLocationSignature<TextDocumentPositionParams, Location>[]> {
        return this.providersForDocument(document, registrationOptions => registrationOptions.id === id)
    }

    /**
     * Gets locations from the provider with the given ID (i.e., the `id` parameter to
     * {@link sourcegraph.languageFeatures.registerLocationProvider}).
     *
     * @param id The provider ID.
     */
    public getLocation(id: string, params: TextDocumentPositionParams): Observable<Location[] | null> {
        return getLocations(this.providersForDocumentWithID(id, params.textDocument), params)
    }
}
