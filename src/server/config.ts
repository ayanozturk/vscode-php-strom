import { ClientCapabilities } from 'vscode-languageserver/node';

export interface DiagnosticsOverride {
  classes?: string[];
}

export interface DiagnosticsAnalysisConfig {
  syntaxErrors: boolean;
  undefinedSymbols: boolean;
  undefinedVariables: boolean;
  classModel: boolean;
  invalidCalls: boolean;
  language: boolean;
  typeErrors: boolean;
  methodVisibility: boolean;
  throwTypes: boolean;
  deprecated: boolean;
  unreachableCode: boolean;
  emptyStatements: boolean;
  assignmentInCondition: boolean;
  sideEffects: boolean;
  style: boolean;
}

export interface DiagnosticsConfig {
  enable: boolean;
  run: 'onType' | 'onSave';
  undefinedSymbols: boolean;
  undefinedVariables: boolean;
  typeErrors: boolean;
  analysis: DiagnosticsAnalysisConfig;
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
      associations: ['**/*.php', '**/*.phar'], // Only pure PHP files by default
      exclude: [
        '**/.git/**',
        '**/node_modules/**',
        '**/*.phtml',
        '**/*.tpl',
        '**/*.html',
        '**/*.htm',
        '**/*.php4',
        '**/*.php5',
      ],
      maxSize: 1_000_000,
    };
    this.stubs = [];
    this.diagnostics = {
      enable: true,
      run: 'onType',
      undefinedSymbols: true,
      undefinedVariables: true,
      typeErrors: true,
      analysis: {
        syntaxErrors: true,
        undefinedSymbols: true,
        undefinedVariables: true,
        classModel: true,
        invalidCalls: true,
        language: true,
        typeErrors: true,
        methodVisibility: true,
        throwTypes: true,
        deprecated: true,
        unreachableCode: true,
        emptyStatements: true,
        assignmentInCondition: true,
        sideEffects: false,
        style: false,
      },
      exclude: {
        // Exclude template/mixed-content files from diagnostics
        '**/*.phtml': ['*'],
        '**/*.tpl': ['*'],
        '**/*.html': ['*'],
        '**/*.htm': ['*'],
        '**/*.php4': ['*'],
        '**/*.php5': ['*'],
        'vendor/**': ['*'], // Exclude all vendor files from diagnostics, but still index for symbol resolution
        // Note: .php files containing <html or <!DOCTYPE html are also excluded by a heuristic in DiagnosticsProvider
      },
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
      parameterTypes: false,
      returnTypes: false,
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
