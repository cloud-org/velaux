import React from 'react';

import type { WorkflowMode } from '../interface/application';
import type { WorkflowStep } from '../interface/pipeline';

export interface UISchemaContextState {
  appName?: string;
  appNamespace?: string;
  projectName?: string;
  envName?: string;
}

export const UISchemaContext = React.createContext<UISchemaContextState>({});

export type WorkflowData = {
  alias?: string;
  name: string;
  description?: string;
  createTime?: string;
  steps: WorkflowStep[];
  mode: WorkflowMode;
  subMode: WorkflowMode;
};

type workflowContext = {
  appName?: string;
  projectName?: string;
  workflow?: WorkflowData;
};

export const WorkflowContext = React.createContext<workflowContext>({});

type workflowEditContext = {
  steps?: WorkflowStep[];
  stepName?: string;
};

export const WorkflowEditContext = React.createContext<workflowEditContext>({});
