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

}

