import type { DemoConfig } from "../util/config.js";
import { log } from "../util/logger.js";
import { type DynamicScalars, setDynamic } from "./scalars.js";
import type { DemoStructures } from "./structures.js";

export function startSimulation(
  scalars: DynamicScalars,
  structures: DemoStructures,
  cfg: DemoConfig,
): { stop: () => void } {
  const start = Date.now();
  let counter = 0;
  let saw = 0;

  const tick = (): void => {
    const t = (Date.now() - start) / 1000;

    counter += 1;
    setDynamic(scalars.counter, counter);

    setDynamic(scalars.sine, Math.sin(2 * Math.PI * 0.5 * t));

    saw = (saw + 0.05) % 1;
    setDynamic(scalars.sawtooth, saw);

    setDynamic(scalars.random, Math.random());

    // Temperature: slow drift around 20°C with small noise
    setDynamic(
      scalars.temperature,
      20 + 2 * Math.sin(2 * Math.PI * 0.05 * t) + (Math.random() - 0.5) * 0.2,
    );

    // Pressure: 1 bar baseline with occasional spike — useful for deadband tests
    const spike = Math.random() < 0.05 ? Math.random() * 0.4 : 0;
    setDynamic(scalars.pressure, 1.013 + spike + (Math.random() - 0.5) * 0.01);
  };

  const motorTick = (): void => {
    // Motor speed wanders around 1450 RPM, torque follows
    const m = structures.currentMotor;
    m.Speed = 1450 + 50 * Math.sin(Date.now() / 4000) + (Math.random() - 0.5) * 5;
    m.Torque = 12 + 2 * Math.cos(Date.now() / 5000);
    if (Math.random() < 0.001) {
      m.FaultCode = m.FaultCode === 0 ? 42 : 0;
      log.warn(`motor fault toggled: code=${m.FaultCode}`);
    }
  };

  const tickHandle = setInterval(tick, cfg.simulation.tickIntervalMs);
  const motorHandle = setInterval(motorTick, cfg.simulation.motorTickMs);

  log.info(
    `simulation running: tick=${cfg.simulation.tickIntervalMs}ms motor=${cfg.simulation.motorTickMs}ms`,
  );

  return {
    stop: () => {
      clearInterval(tickHandle);
      clearInterval(motorHandle);
    },
  };
}
