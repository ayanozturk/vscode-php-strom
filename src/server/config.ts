import { ClientCapabilities } from 'vscode-languageserver/node';

export interface DiagnosticsOverride {
  classes?: string[];
}

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
  overrides: Record<string, DiagnosticsOverride>;
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
      overrides: {},
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
    const phpstrom = (settings['phpstrom'] ?? settings) as Record<string, unknown>;

    if (phpstrom['environment']) {
      Object.assign(this.environment, phpstrom['environment']);
    }
    if (phpstrom['files']) {
      Object.assign(this.files, phpstrom['files']);
    }
    if (phpstrom['stubs']) {
      this.stubs = phpstrom['stubs'] as string[];
    }
    if (phpstrom['diagnostics']) {
      Object.assign(this.diagnostics, phpstrom['diagnostics']);
    }
    if (phpstrom['completion']) {
      Object.assign(this.completion, phpstrom['completion']);
    }
    if (phpstrom['format']) {
      Object.assign(this.format, phpstrom['format']);
    }
    if (phpstrom['phpdoc']) {
      Object.assign(this.phpdoc, phpstrom['phpdoc']);
    }
    if (phpstrom['codeLens']) {
      const cl = phpstrom['codeLens'] as Record<string, Record<string, boolean>>;
      this.codeLens.references = cl['references']?.['enable'] ?? this.codeLens.references;
      this.codeLens.implementations = cl['implementations']?.['enable'] ?? this.codeLens.implementations;
      this.codeLens.overrides = cl['overrides']?.['enable'] ?? this.codeLens.overrides;
      this.codeLens.parent = cl['parent']?.['enable'] ?? this.codeLens.parent;
      this.codeLens.usages = cl['usages']?.['enable'] ?? this.codeLens.usages;
    }
    if (phpstrom['inlayHints']) {
      const ih = phpstrom['inlayHints'] as Record<string, Record<string, boolean>>;
      this.inlayHints.parameterNames = ih['parameterNames']?.['enable'] ?? this.inlayHints.parameterNames;
      this.inlayHints.parameterTypes = ih['parameterTypes']?.['enable'] ?? this.inlayHints.parameterTypes;
      this.inlayHints.returnTypes = ih['returnTypes']?.['enable'] ?? this.inlayHints.returnTypes;
    }
    if (phpstrom['compatibility']) {
      const compat = phpstrom['compatibility'] as Record<string, boolean>;
      this.preferPsalmPhpstanPrefixedAnnotations =
        compat['preferPsalmPhpstanPrefixedAnnotations'] ?? this.preferPsalmPhpstanPrefixedAnnotations;
    }
  }
}
