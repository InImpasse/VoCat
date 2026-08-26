export interface AutomaticTaskWrite {
  name: string;
  enabled: boolean;
  deviceId: string;
  profileIccid: string;
  profileAid: string;
  taskType: "sms" | "call" | "public_ip";
  environment: "vowifi" | "cellular";
  intervalDays: number;
  startDate: string;
  runTime: string;
  timezone: string;
  payload: {
    phone?: string;
    message?: string;
    durationSeconds?: number;
  };
  retryCount: number;
  notify: boolean;
}

export function automaticTaskUpdate(task: AutomaticTaskWrite, enabled: boolean): AutomaticTaskWrite {
  return {
    name: task.name,
    enabled,
    deviceId: task.deviceId,
    profileIccid: task.profileIccid,
    profileAid: task.profileAid,
    taskType: task.taskType,
    environment: task.environment,
    intervalDays: task.intervalDays,
    startDate: task.startDate,
    runTime: task.runTime,
    timezone: task.timezone,
    payload: task.payload,
    retryCount: task.retryCount,
    notify: task.notify,
  };
}
