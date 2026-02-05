// Package gateway provides the gateway service for single-port sandbox access.
//
// The gateway enables SSH access to multiple sandboxes through a single entry
// point, simplifying network configuration and providing an interactive sandbox
// picker.
//
// # Architecture
//
// The gateway runs on the host and accepts SSH connections on a single port.
// Users can either specify a sandbox name directly or use an interactive picker.
//
// # Usage
//
// As SSH ForceCommand:
//
//	# In sshd_config:
//	Match User forage
//	    ForceCommand /run/current-system/sw/bin/forage-ctl gateway
//
// Client connection:
//
//	ssh forage@host sandbox-name  # Direct connection
//	ssh forage@host               # Interactive picker
//
// # Server
//
// The Server type handles gateway operations:
//
//	server := gateway.NewServer(paths)
//	server.HandleSSHOriginalCommand()  // Process SSH_ORIGINAL_COMMAND
//	server.HandleConnection(args)      // Handle connection with args
//	server.ConnectToSandbox(name)      // Connect to specific sandbox
//	server.ShowPicker()                // Show interactive picker
package gateway
