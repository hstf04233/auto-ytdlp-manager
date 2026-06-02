
function truncateString(str, num) {
	if (str.length > num) {
		return str.slice(0, num) + "...";
	} else {
		return str;
	}
}

function escHtml(str) {
	if (!str) return '';
	const div = document.createElement('div');
	div.textContent = str;
	return div.innerHTML;
}

// ========== API helpers ==========
const API = {
	async get(url) {
		const res = await fetch(url);
		if (!res.ok) {
			const text = await res.text();
			throw new Error(`API GET: ${res.status} - ${text}`);
		}
		return res.json();
	},
	async post(url, body) {
		const res = await fetch(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body),
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(`API POST: ${res.status} - ${text}`);
		}
		return res.json();
	},
	async patch(url, body) {
		const res = await fetch(url, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body),
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(`API PATCH: ${res.status} - ${text}`);
		}
		return res.json();
	},
	async del(url) {
		const res = await fetch(url, {
			method: 'DELETE',
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(`API DELETE: ${res.status} - ${text}`);
		}
		return res.json();
	},
};

let createUserMode = 0;

function closeCreateUserPrompt() {
	createUserModal.classList.remove("active");
}

function promptCreateUser(createAdmin) {
	const createUserModal = document.getElementById("createUserModal");
	createUserModal.classList.add("active");
	
	createUserMode = (!createAdmin ? 0 : 1);
	
	const createUserTitle = document.getElementById("createUserTitle");
	if (createUserTitle) {
		createUserTitle.textContent = (!createAdmin ? "Create user" : "Create admin account");
	}
	const createUserLabel = document.getElementById("createUserLabel");
	if (createUserLabel) {
		createUserLabel.textContent = (!createAdmin ? "Create a user account" : "No admin account yet. Create the admin account!");
	}
	
	const username = document.getElementById("createUser-Username");
	if (username) {
		username.value = "";
	}
	const password = document.getElementById("createUser-Password");
	if (password) {
		password.value = "";
	}
	const passwordRetype = document.getElementById("createUser-passwordRetype");
	if (passwordRetype) {
		passwordRetype.value = "";
	}
	
	SubmitBtn = document.getElementById("createUser-SubmitBtn");
	if (SubmitBtn) {
		SubmitBtn.textContent = (!createAdmin ? "Create user" : "Create admin account");
	}
}

function createUserStatus(msg) {
	const status = document.getElementById("createUser-Status");
	status.textContent = msg;
	
	const statusContainer = document.getElementById("createUser-StatusContainer");
	statusContainer.classList.add("active");
}
function loginStatus(msg) {
	const status = document.getElementById("login-Status");
	status.textContent = msg;
	
	const statusContainer = document.getElementById("login-StatusContainer");
	statusContainer.classList.add("active");
}

async function createUser(e) {
	e.preventDefault();
	
	const username = document.getElementById("createUser-Username").value.trim();
	const password = document.getElementById("createUser-Password").value;
	const passwordRetype = document.getElementById("createUser-PasswordRetype").value;
	
	if (password !== passwordRetype) {
		createUserStatus("Passwords do not match! Please type the same password for both fields.");
		return;
	}
	
	try {
		const body = {
			username: username,
			password: password,
		};
		
		if (createUserMode == 0) {
			await API.post('/api/create-user', body);
		} else {
			await API.post('/api/create-admin', body);
		}
		
		loginStatus("Account created successfully! Please log in to the account.")
		
		closeCreateUserPrompt();
	} catch (err) {
		createUserStatus(`Failed to create account: ${err.message}`);
	}
}

async function login(e) {
	e.preventDefault();
	
	const username = document.getElementById("loginUsername").value.trim();
	const password = document.getElementById("loginPassword").value;
	
	try {
		const body = {
			username: username,
			password: password,
		};
		
		//await API.post('/api/login', body);
		const res = await fetch('/api/login', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(body),
		});
		if (!res.ok) {
			const text = await res.text();
			throw new Error(text);
		}
		const responseData = res.json();
		
		loginStatus("Log in successful!")
		
		window.location.href = "/";
	} catch (err) {
		loginStatus(`Failed to log in: ${err.message}`);
	}
}

async function doesAdminAccountExist() {
	try {
		const data = await API.get('/api/admin-account-exists');
		
		if (data.exists) {
			// Admin account exists.
			// Don't do anything!
			return;
		}
		
		// Admin account does not exist...
		// Prompt to create admin account
		promptCreateUser(true);
	} catch (err) {
		console.log(`Failed to check if admin account exists... ${err.message}`)
	}
}

async function init() {
	doesAdminAccountExist();
}

init()
