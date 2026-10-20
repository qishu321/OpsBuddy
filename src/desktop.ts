import type { Asset, DirectoryGrant, ProjectImportResult } from './api';
export interface PetSettings {
  enabled: boolean;
  animate: boolean;
  topmost: boolean;
  x: number;
  y: number;
  positioned: boolean;
}

interface NativeAPI {
  ImportProject(): Promise<ProjectImportResult>;
  PetSettings(): Promise<PetSettings>;
  ConfigurePet(settings: PetSettings): Promise<PetSettings>;
  PetDiagnostics(): Promise<{
    visible: boolean;
    timerActive: boolean;
    renders: number;
    nativeWindow: boolean;
    phase: string;
    stage: string;
    reactions: number;
    dpi: number;
    fontPixels: number;
  }>;
  Ready(): Promise<void>;
  ReportSmoke(report: string): Promise<void>;
  PickFiles(projectID: string): Promise<Asset[]>;
  PickFolder(projectID: string): Promise<Asset[]>;
  PickScheduleDirectory(projectID: string): Promise<DirectoryGrant>;
  SaveAsset(assetID: string): Promise<string>;
  OpenAsset(assetID: string): Promise<void>;
  RevealAsset(assetID: string): Promise<void>;
  OpenDataFolder(): Promise<void>;
  Info(): Promise<{
    mode: string;
    version: string;
    platform?: string;
    petSupported?: string;
    dataDirectory: string;
    smokeTest: string;
    smokeModelURL?: string;
    smokeImportDemo?: string;
  }>;
}
declare global {
  interface Window {
    go?: { desktop?: { NativeAPI?: NativeAPI } };
  }
}
export const desktop = () => window.go?.desktop?.NativeAPI;
export const isDesktop = () => Boolean(desktop());
