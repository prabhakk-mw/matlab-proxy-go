% enable_connect.m — Prepare a running MATLAB session for matlab-proxy attach mode.
%
% Run this script in your MATLAB command window, then start matlab-proxy with
% the printed --ec-port and --mwapikey values.
%
% Copyright 2026 The MathWorks, Inc.

disp('[matlab-proxy] Setting up attach mode...');

% Create a temporary directory for the Embedded Connector port file
mwi_logDir = fullfile(tempdir, 'matlab-proxy-attach');
if ~exist(mwi_logDir, 'dir')
    mkdir(mwi_logDir);
end
setenv('MATLAB_LOG_DIR', mwi_logDir);
disp(['[matlab-proxy] Log directory: ' mwi_logDir]);

% Generate a unique API key for authenticating with the Embedded Connector
mwi_key = lower(sprintf('%s-%s-%s-%s-%s', ...
    dec2hex(randi([0 2^32-1], 1, 1), 8), ...
    dec2hex(randi([0 2^16-1], 1, 1), 4), ...
    dec2hex(randi([0 2^16-1], 1, 1), 4), ...
    dec2hex(randi([0 2^16-1], 1, 1), 4), ...
    dec2hex(randi([0 2^48-1], 1, 1), 12)));
setenv('MWAPIKEY', mwi_key);
disp('[matlab-proxy] Generated API key.');

% Set the connector context root (empty string = default "/")
setenv('MW_CONNECTOR_CONTEXT_ROOT', '');

% Set the document root so the EC can serve the web desktop files
setenv('MW_DOCROOT', fullfile('ui', 'webgui', 'src'));

% Enable change-directory support
setenv('MW_CD_ANYWHERE_ENABLED', 'true');
setenv('MW_CD_ANYWHERE_DISABLED', 'false');

disp('[matlab-proxy] Configured Embedded Connector environment.');

% Start the Embedded Connector (may clear the workspace)
disp('[matlab-proxy] Starting Embedded Connector...');
evalc('connector.internal.Worker.start');

% Recover values from environment variables (survives workspace clear)
mwi_logDir = getenv('MATLAB_LOG_DIR');
mwi_key = getenv('MWAPIKEY');

% Wait for the connector to write its port file (up to 30 seconds)
mwi_portFile = fullfile(mwi_logDir, 'connector.securePort');
disp('[matlab-proxy] Waiting for Embedded Connector port file...');
for mwi_i = 1:60
    if isfile(mwi_portFile)
        break;
    end
    if mod(mwi_i, 10) == 0
        disp(['[matlab-proxy] Still waiting... (' num2str(mwi_i / 2) ' seconds elapsed)']);
    end
    pause(0.5);
end

if ~isfile(mwi_portFile)
    error('matlab-proxy:enable_connect', ...
        'Timed out waiting for Embedded Connector to start. Port file not found: %s', mwi_portFile);
end

mwi_port = strip(fileread(mwi_portFile));
disp(['[matlab-proxy] Embedded Connector is running on port ' mwi_port '.']);

disp(' ');
disp('==========================================================================');
disp('  MATLAB is ready for matlab-proxy. Start the proxy with:');
disp(' ');
disp(['    matlab-proxy --ec-port ' mwi_port ' --mwapikey ' mwi_key]);
disp(' ');
disp('  Or using environment variables:');
disp(' ');
disp(['    MWI_ATTACH_EC_PORT=' mwi_port ' MWI_ATTACH_MWAPIKEY=' mwi_key ' matlab-proxy']);
disp('==========================================================================');
disp(' ');

clear mwi_logDir mwi_key mwi_portFile mwi_port mwi_i
