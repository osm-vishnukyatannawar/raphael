export namespace billing {
	
	export class DayTotal {
	    day: string;
	    billableMinutes: number;
	    nonBillableMinutes: number;
	
	    static createFrom(source: any = {}) {
	        return new DayTotal(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.billableMinutes = source["billableMinutes"];
	        this.nonBillableMinutes = source["nonBillableMinutes"];
	    }
	}
	export class Summary {
	    todayMinutes: number;
	    yesterdayMinutes: number;
	    weekMinutes: number;
	    weekBillableMinutes: number;
	    weekNonBillableMinutes: number;
	    weekStart: string;
	    weekEnd: string;
	    days: DayTotal[];
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.todayMinutes = source["todayMinutes"];
	        this.yesterdayMinutes = source["yesterdayMinutes"];
	        this.weekMinutes = source["weekMinutes"];
	        this.weekBillableMinutes = source["weekBillableMinutes"];
	        this.weekNonBillableMinutes = source["weekNonBillableMinutes"];
	        this.weekStart = source["weekStart"];
	        this.weekEnd = source["weekEnd"];
	        this.days = this.convertValues(source["days"], DayTotal);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace identity {
	
	export class Session {
	    displayName: string;
	    userName: string;
	    firstName: string;
	    lastName: string;
	    companyId: number;
	    companyName: string;
	    isProjectManager: boolean;
	    isTeamLead: boolean;
	    secretsInKeyring: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.displayName = source["displayName"];
	        this.userName = source["userName"];
	        this.firstName = source["firstName"];
	        this.lastName = source["lastName"];
	        this.companyId = source["companyId"];
	        this.companyName = source["companyName"];
	        this.isProjectManager = source["isProjectManager"];
	        this.isTeamLead = source["isTeamLead"];
	        this.secretsInKeyring = source["secretsInKeyring"];
	    }
	}

}

export namespace main {
	
	export class AppInfo {
	    name: string;
	    version: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.platform = source["platform"];
	    }
	}
	export class BillingResult {
	    summary?: billing.Summary;
	    syncedAt: string;
	    errorMessage: string;
	    fromCacheOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BillingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], billing.Summary);
	        this.syncedAt = source["syncedAt"];
	        this.errorMessage = source["errorMessage"];
	        this.fromCacheOnly = source["fromCacheOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class BootstrapResult {
	    info: AppInfo;
	    onboarded: boolean;
	    session?: identity.Session;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new BootstrapResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.info = this.convertValues(source["info"], AppInfo);
	        this.onboarded = source["onboarded"];
	        this.session = this.convertValues(source["session"], identity.Session);
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SignInResult {
	    session?: identity.Session;
	    invalidLogin: boolean;
	    errorMessage: string;
	    suggestedName: string;
	
	    static createFrom(source: any = {}) {
	        return new SignInResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = this.convertValues(source["session"], identity.Session);
	        this.invalidLogin = source["invalidLogin"];
	        this.errorMessage = source["errorMessage"];
	        this.suggestedName = source["suggestedName"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TasksResult {
	    tasks: tasks.Task[];
	    syncedAt: string;
	    errorMessage: string;
	    fromCacheOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TasksResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tasks = this.convertValues(source["tasks"], tasks.Task);
	        this.syncedAt = source["syncedAt"];
	        this.errorMessage = source["errorMessage"];
	        this.fromCacheOnly = source["fromCacheOnly"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace settings {
	
	export class Settings {
	    refreshIntervalSeconds: number;
	    billingRefreshIntervalSeconds: number;
	    weekStartDay: number;
	    notifyNewTasks: boolean;
	    focusOnNewTask: boolean;
	    tasksSyncedAt: string;
	    billingSyncedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refreshIntervalSeconds = source["refreshIntervalSeconds"];
	        this.billingRefreshIntervalSeconds = source["billingRefreshIntervalSeconds"];
	        this.weekStartDay = source["weekStartDay"];
	        this.notifyNewTasks = source["notifyNewTasks"];
	        this.focusOnNewTask = source["focusOnNewTask"];
	        this.tasksSyncedAt = source["tasksSyncedAt"];
	        this.billingSyncedAt = source["billingSyncedAt"];
	    }
	}

}

export namespace tasks {
	
	export class Task {
	    taskId: number;
	    shortCode: string;
	    name: string;
	    projectCode: string;
	    projectName: string;
	    priority: string;
	    statusType: string;
	    statusColor: string;
	    dueDate: string;
	    modifiedOn: string;
	    sprintName: string;
	    competencyName: string;
	    isNew: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.taskId = source["taskId"];
	        this.shortCode = source["shortCode"];
	        this.name = source["name"];
	        this.projectCode = source["projectCode"];
	        this.projectName = source["projectName"];
	        this.priority = source["priority"];
	        this.statusType = source["statusType"];
	        this.statusColor = source["statusColor"];
	        this.dueDate = source["dueDate"];
	        this.modifiedOn = source["modifiedOn"];
	        this.sprintName = source["sprintName"];
	        this.competencyName = source["competencyName"];
	        this.isNew = source["isNew"];
	    }
	}

}

