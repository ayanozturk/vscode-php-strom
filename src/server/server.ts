import {
  createConnection,
  ProposedFeatures,
  InitializeParams,
  InitializeResult,
  TextDocumentSyncKind,
  DidOpenTextDocumentParams,
  DidChangeTextDocumentParams,
  DidCloseTextDocumentParams,
  DidSaveTextDocumentParams,
  TextDocumentPositionParams,
  CompletionParams,
  ReferenceParams,
  CodeActionParams,
  CodeLensParams,
  DocumentLinkParams,
  DocumentFormattingParams,
  DocumentRangeFormattingParams,
  FoldingRangeParams,
  SelectionRangeParams,
  InlayHintParams,
  TypeHierarchyPrepareParams,
  WorkspaceSymbolParams,
  DocumentSymbolParams,
  RenameParams,
  PrepareRenameParams,
  ImplementationParams,
  TypeDefinitionParams,
  DeclarationParams,
  DocumentHighlightParams,
  HoverParams,
  SignatureHelpParams,
  DefinitionParams,
} from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { WorkspaceIndexer } from './indexer/workspaceIndexer.js';
import { ServerConfig } from './config.js';
import { CompletionProvider } from './providers/completionProvider.js';
import { DefinitionProvider } from './providers/definitionProvider.js';
import { HoverProvider } from './providers/hoverProvider.js';
import { DiagnosticsProvider } from './providers/diagnosticsProvider.js';
import { ReferencesProvider } from './providers/referencesProvider.js';
import { SymbolProvider } from './providers/symbolProvider.js';
import { SignatureHelpProvider } from './providers/signatureHelpProvider.js';
import { FormattingProvider } from './providers/formattingProvider.js';
import { RenameProvider } from './providers/renameProvider.js';
import { FoldingProvider } from './providers/foldingProvider.js';
import { ImplementationProvider } from './providers/implementationProvider.js';
import { TypeDefinitionProvider } from './providers/typeDefinitionProvider.js';
import { DeclarationProvider } from './providers/declarationProvider.js';
import { SelectionRangeProvider } from './providers/selectionRangeProvider.js';
import { CodeActionProvider } from './providers/codeActionProvider.js';
import { CodeLensProvider } from './providers/codeLensProvider.js';
import { InlayHintsProvider } from './providers/inlayHintsProvider.js';
import { DocumentLinksProvider } from './providers/documentLinksProvider.js';
import { TypeHierarchyProvider } from './providers/typeHierarchyProvider.js';
import { HighlightProvider } from './providers/highlightProvider.js';

const connection = createConnection(ProposedFeatures.all);

// Document store
const documents = new Map<string, TextDocument>();

let indexer: WorkspaceIndexer;
let config: ServerConfig;

// Providers (instantiated after initialize)
let completionProvider: CompletionProvider;
let definitionProvider: DefinitionProvider;
let hoverProvider: HoverProvider;
let diagnosticsProvider: DiagnosticsProvider;
let referencesProvider: ReferencesProvider;
let symbolProvider: SymbolProvider;
let signatureHelpProvider: SignatureHelpProvider;
let formattingProvider: FormattingProvider;
let renameProvider: RenameProvider;
let foldingProvider: FoldingProvider;
let implementationProvider: ImplementationProvider;
let typeDefinitionProvider: TypeDefinitionProvider;
let declarationProvider: DeclarationProvider;
let selectionRangeProvider: SelectionRangeProvider;
let codeActionProvider: CodeActionProvider;
let codeLensProvider: CodeLensProvider;
let inlayHintsProvider: InlayHintsProvider;
let documentLinksProvider: DocumentLinksProvider;
let typeHierarchyProvider: TypeHierarchyProvider;
let highlightProvider: HighlightProvider;

// ─── Initialize ──────────────────────────────────────────────────────────────

connection.onInitialize((params: InitializeParams): InitializeResult => {
  config = new ServerConfig(params.initializationOptions, params.capabilities);
  indexer = new WorkspaceIndexer(connection, config);

  // Wire all providers against the shared indexer
  completionProvider = new CompletionProvider(indexer, config);
  definitionProvider = new DefinitionProvider(indexer);
  hoverProvider = new HoverProvider(indexer);
  diagnosticsProvider = new DiagnosticsProvider(indexer, config, connection);
  referencesProvider = new ReferencesProvider(indexer);
  symbolProvider = new SymbolProvider(indexer);
  signatureHelpProvider = new SignatureHelpProvider(indexer);
  formattingProvider = new FormattingProvider(config);
  renameProvider = new RenameProvider(indexer);
  foldingProvider = new FoldingProvider(indexer);
  implementationProvider = new ImplementationProvider(indexer);
  typeDefinitionProvider = new TypeDefinitionProvider(indexer);
  declarationProvider = new DeclarationProvider(indexer);
  selectionRangeProvider = new SelectionRangeProvider(indexer);
  codeActionProvider = new CodeActionProvider(indexer, config);
  codeLensProvider = new CodeLensProvider(indexer, config);
  inlayHintsProvider = new InlayHintsProvider(indexer, config);
  documentLinksProvider = new DocumentLinksProvider(indexer, config);
  typeHierarchyProvider = new TypeHierarchyProvider(indexer);
  highlightProvider = new HighlightProvider(indexer);

  const result: InitializeResult = {
    capabilities: {
      textDocumentSync: {
        openClose: true,
        change: TextDocumentSyncKind.Incremental,
        save: { includeText: false },
      },
      // Free features
      completionProvider: {
        triggerCharacters: ['$', '>', ':', '\\', '/', "'", '"', '*', '.', '<'],
        resolveProvider: true,
      },
      signatureHelpProvider: {
        triggerCharacters: ['(', ',', ':'],
        retriggerCharacters: [','],
      },
      hoverProvider: true,
      definitionProvider: true,
      referencesProvider: true,
      documentHighlightProvider: true,
      documentSymbolProvider: true,
      workspaceSymbolProvider: true,
      diagnosticProvider: {
        interFileDependencies: true,
        workspaceDiagnostics: false,
      },
      documentFormattingProvider: true,
      documentRangeFormattingProvider: true,
      inlineValueProvider: true,

      // Premium-equivalent features (all free in phpstrom)
      renameProvider: { prepareProvider: true },
      foldingRangeProvider: true,
      implementationProvider: true,
      typeDefinitionProvider: true,
      declarationProvider: true,
      selectionRangeProvider: true,
      codeActionProvider: {
        codeActionKinds: [
          'quickfix',
          'refactor',
          'source.organizeImports',
        ],
        resolveProvider: true,
      },
      codeLensProvider: { resolveProvider: true },
      inlayHintProvider: { resolveProvider: false },
      documentLinkProvider: { resolveProvider: false },
      typeHierarchyProvider: true,
    },
    serverInfo: {
      name: 'phpstrom',
      version: '0.1.0',
    },
  };

  return result;
});

connection.onInitialized(async () => {
  connection.sendNotification('phpstrom/indexingStarted');
  await indexer.indexWorkspace();
  connection.sendNotification('phpstrom/indexingFinished');
});

// ─── Document Lifecycle ──────────────────────────────────────────────────────

connection.onDidOpenTextDocument((params: DidOpenTextDocumentParams) => {
  const doc = TextDocument.create(
    params.textDocument.uri,
    params.textDocument.languageId,
    params.textDocument.version,
    params.textDocument.text,
  );
  documents.set(params.textDocument.uri, doc);
  indexer.onDocumentOpen(doc);
  if (config.diagnostics.run === 'onType') {
    diagnosticsProvider.validate(doc);
  }
});

connection.onDidChangeTextDocument((params: DidChangeTextDocumentParams) => {
  const existing = documents.get(params.textDocument.uri);
  if (!existing) return;
  const updated = TextDocument.update(existing, params.contentChanges, params.textDocument.version);
  documents.set(params.textDocument.uri, updated);
  indexer.onDocumentChange(updated);
  if (config.diagnostics.run === 'onType') {
    diagnosticsProvider.validate(updated);
  }
});

connection.onDidSaveTextDocument((params: DidSaveTextDocumentParams) => {
  const doc = documents.get(params.textDocument.uri);
  if (!doc) return;
  diagnosticsProvider.validate(doc);
});

connection.onDidCloseTextDocument((params: DidCloseTextDocumentParams) => {
  documents.delete(params.textDocument.uri);
  indexer.onDocumentClose(params.textDocument.uri);
  connection.sendDiagnostics({ uri: params.textDocument.uri, diagnostics: [] });
});

// ─── LSP Request Handlers ────────────────────────────────────────────────────

connection.onCompletion((params: CompletionParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? completionProvider.provide(doc, params.position, params.context) : null;
});

connection.onCompletionResolve((item) => completionProvider.resolve(item));

connection.onHover((params: HoverParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? hoverProvider.provide(doc, params.position) : null;
});

connection.onDefinition((params: DefinitionParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? definitionProvider.provide(doc, params.position) : null;
});

connection.onReferences((params: ReferenceParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? referencesProvider.provide(doc, params.position, params.context) : null;
});

connection.onDocumentHighlight((params: DocumentHighlightParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? highlightProvider.provide(doc, params.position) : null;
});

connection.onDocumentSymbol((params: DocumentSymbolParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? symbolProvider.provideDocumentSymbols(doc) : null;
});

connection.onWorkspaceSymbol((params: WorkspaceSymbolParams) => {
  return symbolProvider.provideWorkspaceSymbols(params.query);
});

connection.onSignatureHelp((params: SignatureHelpParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? signatureHelpProvider.provide(doc, params.position, params.context) : null;
});

connection.onDocumentFormatting((params: DocumentFormattingParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? formattingProvider.format(doc, params.options) : null;
});

connection.onDocumentRangeFormatting((params: DocumentRangeFormattingParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? formattingProvider.formatRange(doc, params.range, params.options) : null;
});

// Premium-equivalent handlers (all free in phpstrom)

connection.onRenameRequest((params: RenameParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? renameProvider.provide(doc, params.position, params.newName) : null;
});

connection.onPrepareRename((params: PrepareRenameParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? renameProvider.prepare(doc, params.position) : null;
});

connection.onFoldingRanges((params: FoldingRangeParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? foldingProvider.provide(doc) : null;
});

connection.onImplementation((params: ImplementationParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? implementationProvider.provide(doc, params.position) : null;
});

connection.onTypeDefinition((params: TypeDefinitionParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? typeDefinitionProvider.provide(doc, params.position) : null;
});

connection.onDeclaration((params: DeclarationParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? declarationProvider.provide(doc, params.position) : null;
});

connection.onSelectionRanges((params: SelectionRangeParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? selectionRangeProvider.provide(doc, params.positions) : null;
});

connection.onCodeAction((params: CodeActionParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? codeActionProvider.provide(doc, params.range, params.context) : null;
});

connection.onCodeActionResolve((action) => codeActionProvider.resolve(action));

connection.onCodeLens((params: CodeLensParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? codeLensProvider.provide(doc) : null;
});

connection.onCodeLensResolve((lens) => codeLensProvider.resolve(lens));

connection.languages.inlayHint.on((params: InlayHintParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? inlayHintsProvider.provide(doc, params.range) : null;
});

connection.onDocumentLinks((params: DocumentLinkParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? documentLinksProvider.provide(doc) : null;
});

connection.languages.typeHierarchy.onPrepare((params: TypeHierarchyPrepareParams) => {
  const doc = documents.get(params.textDocument.uri);
  return doc ? typeHierarchyProvider.prepare(doc, params.position) : null;
});

connection.languages.typeHierarchy.onSupertypes((params) =>
  typeHierarchyProvider.supertypes(params.item),
);

connection.languages.typeHierarchy.onSubtypes((params) =>
  typeHierarchyProvider.subtypes(params.item),
);

// ─── Custom Notifications ────────────────────────────────────────────────────

connection.onNotification('phpstrom/indexWorkspace', async () => {
  connection.sendNotification('phpstrom/indexingStarted');
  await indexer.indexWorkspace();
  connection.sendNotification('phpstrom/indexingFinished');
});

connection.onNotification('workspace/didChangeConfiguration', (params) => {
  config.update(params.settings);
});

// ─── Start ───────────────────────────────────────────────────────────────────

connection.listen();
