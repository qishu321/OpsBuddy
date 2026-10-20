import { useEffect } from 'react';
import { useNodesInitialized, useReactFlow } from '@xyflow/react';

export function FitWorkflowView({ viewId }: { viewId: string }) {
  const initialized = useNodesInitialized();
  const { fitView } = useReactFlow();
  useEffect(() => {
    if (!initialized) return;
    const frame = requestAnimationFrame(() => {
      void fitView({ padding: 0.22, maxZoom: 1 });
    });
    return () => cancelAnimationFrame(frame);
  }, [initialized, viewId, fitView]);
  return null;
}
