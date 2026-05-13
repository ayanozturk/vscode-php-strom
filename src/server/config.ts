import { ClientCapabilities } from 'vscode-languageserver/node';

export interface DiagnosticsConfig {
  enable: boolean;
  run: 'onType' | 'onSave';
  undefinedSymbols: boolean;
  undefinedVariables: boolean;
  typeErrors: boolean;
  strictTypes: boolean;
  relaxedTypeCheck: boolean;
  noMixedTypeCheck: boolean;
  typeCheckDocumentedTypes: boolean;
  exclude: Record<string, string[]>;
}

export interface CompletionConfig {
  insertUseDeclaration: boolean;
  fullyQualifyGlobalSymbols: boolean;
  triggerParameterHints: boolean;
  maxItems: number;
}

export interface FormatConfig {
  braceStyle: 'per' | 'allman' | 'k&r';
  insertSpaces: boolean;
  tabSize: number;
}

export interface PhpDocConfig {
  useFullyQualifiedNames: boolean;
  returnVoid: boolean;
  textFormat: 'snippet' | 'text';
}

export interface CodeLensConfig {
  references: boolean;
  implementations: boolean;
  overrides: boolean;
  parent: boolean;
  usages: boolean;
}

export interface InlayHintsConfig {
  parameterNames: boolean;
  parameterTypes: boolean;
  returnTypes: boolean;
}

export interface EnvironmentConfig {
  phpVersion: string;
  includePaths: string[];
  documentRoot: string;
}

export interface FilesConfig {
  associations: string[];
  exclude: string[];
  maxSize: number;
}

export interface ServerConfig {
  environment: EnvironmentConfig;
  files: FilesConfig;
  stubs: string[];
  diagnostics: DiagnosticsConfig;
  completion: CompletionConfig;
  format: FormatConfig;
  phpdoc: PhpDocConfig;
  codeLens: CodeLensConfig;
  inlayHints: InlayHintsConfig;
  preferPsalmPhpstanPrefixedAnnotations: boolean;
  storagePath: string;
  globalStoragePath: string;
  clearCache: boolean;
  capabilities: ClientCapabilities;
}

export class ServerConfig implements ServerConfig {
  constructor(
    initializationOptions: Record<string, unknown> = {},
    public capabilities: ClientCapabilities = {},
  ) {
    this.storagePath = (initializationOptions['storagePath'] as string) ?? '';
    this.globalStoragePath = (initializationOptions['globalStoragePath'] as string) ?? '';
    this.clearCache = (initializationOptions['clearCache'] as boolean) ?? false;

    this.environment = {
      phpVersion: '8.3',
      includePaths: [],
      documentRoot: '',
    };
    this.files = {
      associations: ['**/*.php', '**/*.phtml', '**/*.phar'],
      exclude: ['**/.git/**', '**/node_modules/**'],
      maxSize: 1_000_000,
    };
    this.stubs = [];
    this.diagnostics = {
      enable: true,
      run: 'onType',
      undefinedSymbols: true,
      undefinedVariables: true,
      typeErrors: true,
      strictTypes: false,
      relaxedTypeCheck: true,
      noMixedTypeCheck: true,
      typeCheckDocumentedTypes: false,
      exclude: {},
    };
    this.completion = {
      insertUseDeclaration: true,
      fullyQualifyGlobalSymbols: false,
      triggerParameterHints: true,
      maxItems: 100,
    };
    this.format = {
      braceStyle: 'per',
      insertSpaces: true,
      tabSize: 4,
    };
    this.phpdoc = {
      useFullyQualifiedNames: false,
      returnVoid: true,
      textFormat: 'snippet',
    };
    this.codeLens = {
      references: false,
      implementations: false,
      overrides: false,
      parent: false,
      usages: false,
    };
    this.inlayHints = {
      parameterNames: true,
      parameterTypes: true,
      returnTypes: true,
    };
    this.preferPsalmPhpstanPrefixedAnnotations = false;
  }

  update(settings: Record<string, unknown>): void {
    const phpls = (settings['phpls'] ?? settings) as Record<string, unknown>;

    if (phpls['environment']) {
      Object.assign(this.environment, phpls['environment']);
    }
    if (phpls['files']) {
      Object.assign(this.files, phpls['files']);
    }
    if (phpls['stubs']) {
      this.stubs = phpls['stubs'] as string[];
    }
    if (phpls['diagnostics']) {
      Object.assign(this.diagnostics, phpls['diagnostics']);
    }
    if (phpls['completion']) {
      Object.assign(this.completion, phpls['completion']);
    }
    if (phpls['format']) {
      Object.assign(this.format, phpls['format']);
    }
    if (phpls['phpdoc']) {
      Object.assign(this.phpdoc, phpls['phpdoc']);
    }
    if (phpls['codeLens']) {
      const cl = phpls['codeLens'] as Record<string, Record<string, boolean>>;
      this.codeLens.references = cl['references']?.['enable'] ?? this.codeLens.references;
      this.codeLens.implementations = cl['implementations']?.['enable'] ?? this.codeLens.implementations;
      this.codeLens.overrides = cl['overrides']?.['enable'] ?? this.codeLens.overrides;
      this.codeLens.parent = cl['parent']?.['enable'] ?? this.codeLens.parent;
      this.codeLens.usages = cl['usages']?.['enable'] ?? this.codeLens.usages;
    }
    if (phpls['inlayHints']) {
      const ih = phpls['inlayHints'] as Record<string, Record<string, boolean>>;
      this.inlayHints.parameterNames = ih['parameterNames']?.['enable'] ?? this.inlayHints.parameterNames;
      this.inlayHints.parameterTypes = ih['parameterTypes']?.['enable'] ?? this.inlayHints.parameterTypes;
      this.inlayHints.returnTypes = ih['returnTypes']?.['enable'] ?? this.inlayHints.returnTypes;
    }
    if (phpls['compatibility']) {
      const compat = phpls['compatibility'] as Record<string, boolean>;
      this.preferPsalmPhpstanPrefixedAnnotations =
        compat['preferPsalmPhpstanPrefixedAnnotations'] ?? this.preferPsalmPhpstanPrefixedAnnotations;
    }
  }
}
